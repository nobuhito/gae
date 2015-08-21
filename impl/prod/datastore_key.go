// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/dskey"
	"google.golang.org/appengine/datastore"
)

type dsKeyImpl struct {
	*datastore.Key
}

var _ ds.Key = dsKeyImpl{}

func (k dsKeyImpl) Parent() ds.Key   { return dsR2F(k.Key.Parent()) }
func (k dsKeyImpl) Incomplete() bool { return k.Key.Incomplete() }
func (k dsKeyImpl) Valid(allowSpecial bool, aid, ns string) bool {
	return dskey.Valid(k, allowSpecial, aid, ns)
}
func (k dsKeyImpl) PartialValid(aid, ns string) bool {
	return dskey.PartialValid(k, aid, ns)
}

// dsR2F (DS real-to-fake) converts an SDK Key to a ds.Key
func dsR2F(k *datastore.Key) ds.Key {
	if k == nil {
		return nil
	}
	return dsKeyImpl{k}
}

// dsF2R (DS fake-to-real) converts a DSKey back to an SDK *Key.
func dsF2R(k ds.Key) *datastore.Key {
	if k == nil {
		return nil
	}
	if rkey, ok := k.(dsKeyImpl); ok {
		return rkey.Key
	}
	// we should always hit the fast case above, but just in case, safely round
	// trip through the proto encoding.
	rkey, err := datastore.DecodeKey(dskey.Encode(k))
	if err != nil {
		// should never happen in a good program, but it's not ignorable, and
		// passing an error back makes this function too cumbersome (and it causes
		// this `if err != nil { panic(err) }` logic to show up in a bunch of
		// places. Realistically, everything should hit the early exit clause above.
		panic(err)
	}
	return rkey
}

// dsMF2R (DS multi-fake-to-fake) converts a slice of wrapped keys to SDK keys.
func dsMF2R(ks []ds.Key) []*datastore.Key {
	ret := make([]*datastore.Key, len(ks))
	for i, k := range ks {
		ret[i] = dsF2R(k)
	}
	return ret
}