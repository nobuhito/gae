// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package constraints contains production datastore constraints.
package constraints

import (
	"github.com/luci/gae/service/datastore"
)

const (
	// TQMaxAddSize is the maximum number of tasks that can be added in a single
	// AddMulti call.
	TQMaxAddSize = 100
)

// DS returns a datastore.Constraints object for the production datastore.
//
// Rationale:
//	- QueryBatchSize was chosen to be a functional batch query size based on
//	  operational observation.
func DS() datastore.Constraints {
	return datastore.Constraints{
		QueryBatchSize: 200,
		MaxPutSize:     500,
	}
}
