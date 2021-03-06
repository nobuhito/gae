// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

import (
	"errors"
	"strings"
	"sync"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/logging/memlogger"

	"golang.org/x/net/context"
)

var serializationDeterministic = false

type memContextObj interface {
	sync.Locker
	canApplyTxn(m memContextObj) bool
	applyTxn(c context.Context, m memContextObj)

	endTxn()
	mkTxn(*ds.TransactionOptions) memContextObj
}

type memContext []memContextObj

var _ memContextObj = (*memContext)(nil)

func newMemContext(aid string) *memContext {
	return &memContext{
		newTaskQueueData(),
		newDataStoreData(aid),
	}
}

type memContextIdx int

const (
	memContextTQIdx memContextIdx = iota
	memContextDSIdx
)

func (m *memContext) Get(itm memContextIdx) memContextObj {
	return (*m)[itm]
}

func (m *memContext) Lock() {
	for _, itm := range *m {
		itm.Lock()
	}
}

func (m *memContext) Unlock() {
	for i := len(*m) - 1; i >= 0; i-- {
		(*m)[i].Unlock()
	}
}

func (m *memContext) endTxn() {
	for _, itm := range *m {
		itm.endTxn()
	}
}

func (m *memContext) mkTxn(o *ds.TransactionOptions) memContextObj {
	ret := make(memContext, len(*m))
	for i, itm := range *m {
		ret[i] = itm.mkTxn(o)
	}
	return &ret
}

func (m *memContext) canApplyTxn(txnCtxObj memContextObj) bool {
	txnCtx := *txnCtxObj.(*memContext)
	for i := range *m {
		if !(*m)[i].canApplyTxn(txnCtx[i]) {
			return false
		}
	}
	return true
}

func (m *memContext) applyTxn(c context.Context, txnCtxObj memContextObj) {
	txnCtx := *txnCtxObj.(*memContext)
	for i := range *m {
		(*m)[i].applyTxn(c, txnCtx[i])
	}
}

// Use calls UseWithAppID with the appid of "app"
func Use(c context.Context) context.Context {
	return UseWithAppID(c, "dev~app")
}

// UseInfo adds an implementation for:
//   * github.com/luci/gae/service/info
// The application id wil be set to 'aid', and will not be modifiable in this
// context. If 'aid' contains a "~" character, it will be treated as the
// fully-qualified App ID and the AppID will be the string following the "~".
func UseInfo(c context.Context, aid string) context.Context {
	if c.Value(&memContextKey) != nil {
		panic(errors.New("memory.Use: called twice on the same Context"))
	}

	fqAppID := aid
	if parts := strings.SplitN(fqAppID, "~", 2); len(parts) == 2 {
		aid = parts[1]
	}

	memctx := newMemContext(fqAppID)
	c = context.WithValue(c, &memContextKey, memctx)

	return useGI(useGID(c, func(mod *globalInfoData) {
		mod.appID = aid
		mod.fqAppID = fqAppID
	}))
}

// UseWithAppID adds implementations for the following gae services to the
// context:
//   * github.com/luci/gae/service/datastore
//   * github.com/luci/gae/service/info
//   * github.com/luci/gae/service/mail
//   * github.com/luci/gae/service/memcache
//   * github.com/luci/gae/service/taskqueue
//   * github.com/luci/gae/service/user
//   * github.com/luci/luci-go/common/logger (using memlogger)
//
// The application id wil be set to 'aid', and will not be modifiable in this
// context. If 'aid' contains a "~" character, it will be treated as the
// fully-qualified App ID and the AppID will be the string following the "~".
//
// These can be retrieved with the gae.Get functions.
//
// The implementations are all backed by an in-memory implementation, and start
// with an empty state.
//
// Using this more than once per context.Context will cause a panic.
func UseWithAppID(c context.Context, aid string) context.Context {
	c = memlogger.Use(c)
	c = UseInfo(c, aid) // Panics if UseWithAppID is called twice.
	return useMod(useMail(useUser(useTQ(useRDS(useMC(c))))))
}

func cur(c context.Context) (*memContext, bool) {
	if txn := c.Value(&currentTxnKey); txn != nil {
		// We are in a Transaction.
		return txn.(*memContext), true
	}
	return c.Value(&memContextKey).(*memContext), false
}

var (
	memContextKey = "gae:memory:context"
	currentTxnKey = "gae:memory:currentTxn"
)

// weird stuff

// RunInTransaction is here because it's really a service-wide transaction, not
// just in the datastore. TaskQueue behaves differently in a transaction in
// a couple ways, for example.
//
// It really should have been appengine.Context.RunInTransaction(func(tc...)),
// but because it's not, this method is on dsImpl instead to mirror the official
// API.
//
// The fake implementation also differs from the real implementation because the
// fake TaskQueue is NOT backed by the fake Datastore. This is done to make the
// test-access API for TaskQueue better (instead of trying to reconstitute the
// state of the task queue from a bunch of datastore accesses).
func (d *dsImpl) RunInTransaction(f func(context.Context) error, o *ds.TransactionOptions) error {
	if d.data.getDisableSpecialEntities() {
		return errors.New("special entities are disabled. no transactions for you")
	}

	// Keep in separate function for defers.
	loopBody := func(applyForReal bool) error {
		curMC, inTxn := cur(d)
		if inTxn {
			return errors.New("datastore: nested transactions are not supported")
		}

		txnMC := curMC.mkTxn(o)

		defer func() {
			txnMC.Lock()
			defer txnMC.Unlock()

			txnMC.endTxn()
		}()

		if err := f(context.WithValue(d, &currentTxnKey, txnMC)); err != nil {
			return err
		}

		txnMC.Lock()
		defer txnMC.Unlock()

		if applyForReal && curMC.canApplyTxn(txnMC) {
			curMC.applyTxn(d, txnMC)
		} else {
			return ds.ErrConcurrentTransaction
		}
		return nil
	}

	// From GAE docs for TransactionOptions: "If omitted, it defaults to 3."
	attempts := 3
	if o != nil && o.Attempts != 0 {
		attempts = o.Attempts
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if err := loopBody(attempt >= d.data.txnFakeRetry); err != ds.ErrConcurrentTransaction {
			return err
		}
	}
	return ds.ErrConcurrentTransaction
}
