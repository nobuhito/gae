// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastore

import (
	"golang.org/x/net/context"
)

// Transaction is a generic interface used to describe a Datastore transaction.
//
// The nil Transaction represents no transaction context.
//
// TODO: Add some functionality here. Ideas include:
//	- Active() bool: is the transaction currently active?
//	- AffectedGroups() []*ds.Key: list the groups that have been referenced in
//	  this Transaction so far.
type Transaction interface{}

// WithoutTransaction returns a Context that isn't bound to a transaction.
// This may be called even when outside of a transaction, in which case the
// input Context is a valid return value.
//
// This can be useful to perform non-transactional tasks given only a Context
// that is bound to a transaction.
func WithoutTransaction(c context.Context) context.Context {
	raw := Raw(c)
	if t := raw.CurrentTransaction(); t == nil {
		// If we're not in a transaction, return the input Contxt.
		return c
	}
	return raw.WithoutTransaction()
}

// CurrentTransaction returns a reference to the current Transaction, or nil
// if the Context does not have a current Transaction.
func CurrentTransaction(c context.Context) Transaction {
	return Raw(c).CurrentTransaction()
}
