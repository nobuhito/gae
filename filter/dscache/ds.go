// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dscache

import (
	"time"

	ds "github.com/luci/gae/service/datastore"
	mc "github.com/luci/gae/service/memcache"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"

	"golang.org/x/net/context"
)

type dsCache struct {
	ds.RawInterface

	*supportContext
}

var _ ds.RawInterface = (*dsCache)(nil)

func (d *dsCache) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	return d.mutation(keys, func() error {
		return d.RawInterface.DeleteMulti(keys, cb)
	})
}

func (d *dsCache) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) error {
	return d.mutation(keys, func() error {
		return d.RawInterface.PutMulti(keys, vals, cb)
	})
}

func (d *dsCache) GetMulti(keys []*ds.Key, metas ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	lockItems, nonce := d.mkRandLockItems(keys, metas)
	if len(lockItems) == 0 {
		return d.RawInterface.GetMulti(keys, metas, cb)
	}

	if err := mc.Add(d.c, lockItems...); err != nil {
		// Ignore this error. Either we couldn't add them because they exist
		// (so, not an issue), or because memcache is having sad times (in which
		// case we'll see so in the Get which immediately follows this).
	}
	if err := errors.Filter(mc.Get(d.c, lockItems...), mc.ErrCacheMiss); err != nil {
		(log.Fields{log.ErrorKey: err}).Debugf(
			d.c, "dscache: GetMulti: memcache.Get")
	}

	p := d.makeFetchPlan(&facts{keys, metas, lockItems, nonce})

	if !p.empty() {
		// looks like we have something to pull from datastore, and maybe some work
		// to save stuff back to memcache.

		toCas := []mc.Item{}
		j := 0
		err := d.RawInterface.GetMulti(p.toGet, p.toGetMeta, func(pm ds.PropertyMap, err error) error {
			i := p.idxMap[j]
			toSave := p.toSave[j]
			j++

			data := []byte(nil)

			// true: save entity to memcache
			// false: lock entity in memcache forever
			shouldSave := true
			if err == nil {
				p.decoded[i] = pm
				if toSave != nil {
					data = encodeItemValue(pm)
					if len(data) > internalValueSizeLimit {
						shouldSave = false
						log.Warningf(
							d.c, "dscache: encoded entity too big (%d/%d)!",
							len(data), internalValueSizeLimit)
					}
				}
			} else {
				p.lme.Assign(i, err)
				if err != ds.ErrNoSuchEntity {
					return nil // aka continue to the next entry
				}
			}

			if toSave != nil {
				if shouldSave { // save
					mg := metas.GetSingle(i)
					expSecs := ds.GetMetaDefault(mg, CacheExpirationMeta, CacheTimeSeconds).(int64)
					toSave.SetFlags(uint32(ItemHasData))
					toSave.SetExpiration(time.Duration(expSecs) * time.Second)
					toSave.SetValue(data)
				} else {
					// Set a lock with an infinite timeout. No one else should try to
					// serialize this item to memcache until something Put/Delete's it.
					toSave.SetFlags(uint32(ItemHasLock))
					toSave.SetExpiration(0)
					toSave.SetValue(nil)
				}
				toCas = append(toCas, toSave)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if len(toCas) > 0 {
			// we have entries to save back to memcache.
			if err := mc.CompareAndSwap(d.c, toCas...); err != nil {
				(log.Fields{log.ErrorKey: err}).Debugf(
					d.c, "dscache: GetMulti: memcache.CompareAndSwap")
			}
		}
	}

	// finally, run the callback for all of the decoded items and the errors,
	// if any.
	for i, dec := range p.decoded {
		if err := cb(dec, p.lme.GetOne(i)); err != nil {
			return err
		}
	}

	return nil
}

func (d *dsCache) RunInTransaction(f func(context.Context) error, opts *ds.TransactionOptions) error {
	txnState := dsTxnState{}
	err := d.RawInterface.RunInTransaction(func(ctx context.Context) error {
		txnState.reset()
		err := f(context.WithValue(ctx, &dsTxnCacheKey, &txnState))
		if err == nil {
			err = txnState.apply(d.supportContext)
		}
		return err
	}, opts)
	if err == nil {
		txnState.release(d.supportContext)
	}
	return err
}
