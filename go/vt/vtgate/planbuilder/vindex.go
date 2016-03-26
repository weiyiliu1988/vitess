// Copyright 2014, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package planbuilder

import (
	"fmt"

	"github.com/youtube/vitess/go/sqltypes"
)

// This file defines interfaces and registration for vindexes.

// A VCursor is an interface that allows you to execute queries
// in the current context and session of a VTGate request. Vindexes
// can use this interface to execute lookup queries.
type VCursor interface {
	Execute(query string, bindvars map[string]interface{}) (*sqltypes.Result, error)
}

// Vindex defines the interface required to register a vindex.
// Additional to these functions, a vindex also needs
// to satisfy the Unique or NonUnique interface.
type Vindex interface {
	// String returns the name of the Vindex instance.
	// It's used for testing and diagnostics. Use pointer
	// comparison to see if two objects refer to the same
	// Vindex.
	String() string
	// Cost is used by planbuilder to prioritize vindexes.
	// The cost can be 0 if the id is basically a keyspace id.
	// The cost can be 1 if the id can be hashed to a keyspace id.
	// The cost can be 2 or above if the id needs to be looked up
	// from an external data source. These guidelines are subject
	// to change in the future.
	Cost() int

	// Verify must be implented by all vindexes. It should return
	// true if the id can be mapped to the keyspace id.
	Verify(cursor VCursor, id interface{}, ks []byte) (bool, error)
}

// Unique defines the interface for a unique vindex.
// For a vindex to be unique, an id has to map to at most
// one keyspace id.
type Unique interface {
	Map(cursor VCursor, ids []interface{}) ([][]byte, error)
}

// NonUnique defines the interface for a non-unique vindex.
// This means that an id can map to multiple keyspace ids.
type NonUnique interface {
	Map(cursor VCursor, ids []interface{}) ([][][]byte, error)
}

// IsUnique returns true if the Vindex is Unique.
func IsUnique(v Vindex) bool {
	_, ok := v.(Unique)
	return ok
}

// A Reversible vindex is one that can perform a
// reverse lookup from a keyspace id to an id. This
// is optional. If present, VTGate can use it to
// fill column values based on the target keyspace id.
type Reversible interface {
	ReverseMap(cursor VCursor, ks []byte) (interface{}, error)
}

// A Functional vindex is an index that can compute
// the keyspace id from the id without a lookup.
// A Functional vindex is also required to be Unique.
// If it's not unique, we cannot determine the target shard
// for an insert operation.
type Functional interface {
	Unique
}

// A Lookup vindex is one that needs to lookup
// a previously stored map to compute the keyspace
// id from an id. This means that the creation of
// a lookup vindex entry requires a keyspace id as
// input.
// A Lookup vindex need not be unique because the
// keyspace_id, which must be supplied, can be used
// to determine the target shard for an insert operation.
type Lookup interface {
	Create(VCursor, interface{}, []byte) error
	Delete(VCursor, []interface{}, []byte) error
}

// A NewVindexFunc is a function that creates a Vindex based on the
// properties specified in the input map. Every vindex must
// register a NewVindexFunc under a unique vindexType.
type NewVindexFunc func(string, map[string]interface{}) (Vindex, error)

var registry = make(map[string]NewVindexFunc)

// Register registers a vindex under the specified vindexType.
// A duplicate vindexType will generate a panic.
// New vindexes will be created using these functions at the
// time of vschema loading.
func Register(vindexType string, newVindexFunc NewVindexFunc) {
	if _, ok := registry[vindexType]; ok {
		panic(fmt.Sprintf("%s is already registered", vindexType))
	}
	registry[vindexType] = newVindexFunc
}

// CreateVindex creates a vindex of the specified type using the
// supplied params. The type must have been previously registered.
func CreateVindex(vindexType, name string, params map[string]interface{}) (Vindex, error) {
	f, ok := registry[vindexType]
	if !ok {
		return nil, fmt.Errorf("vindexType %s not found", vindexType)
	}
	return f(name, params)
}
