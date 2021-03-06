// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastore

import (
	"fmt"
	"reflect"

	"github.com/luci/gae"

	"github.com/luci/luci-go/common/errors"

	"google.golang.org/appengine/datastore"
)

// These errors are returned by various datastore.Interface methods.
var (
	ErrNoSuchEntity          = datastore.ErrNoSuchEntity
	ErrConcurrentTransaction = datastore.ErrConcurrentTransaction

	// Stop is an alias for "github.com/luci/gae".Stop
	Stop = gae.Stop
)

// MakeErrInvalidKey returns an errors.Annotator instance that wraps an invalid
// key error. Calling IsErrInvalidKey on this Annotator or its derivatives will
// return true.
func MakeErrInvalidKey(reason string, args ...interface{}) *errors.Annotator {
	return errors.Annotate(datastore.ErrInvalidKey, reason, args...)
}

// IsErrInvalidKey tests if a given error is a wrapped datastore.ErrInvalidKey
// error.
func IsErrInvalidKey(err error) bool { return errors.Unwrap(err) == datastore.ErrInvalidKey }

// ErrFieldMismatch is returned when a field is to be loaded into a different
// type than the one it was stored from, or when a field is missing or
// unexported in the destination struct.
// StructType is the type of the struct pointed to by the destination argument
// passed to Get or to Iterator.Next.
type ErrFieldMismatch struct {
	StructType reflect.Type
	FieldName  string
	Reason     string
}

func (e *ErrFieldMismatch) Error() string {
	return fmt.Sprintf("gae: cannot load field %q into a %q: %s",
		e.FieldName, e.StructType, e.Reason)
}
