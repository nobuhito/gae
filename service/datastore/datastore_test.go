// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// adapted from github.com/golang/appengine/datastore

package datastore

import (
	"fmt"
	"testing"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/errors"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func fakeDatastoreFactory(c context.Context) RawInterface {
	i := info.Get(c)
	return &fakeDatastore{
		aid: i.FullyQualifiedAppID(),
		ns:  i.GetNamespace(),
	}
}

type fakeQuery struct {
	Query

	limit int

	err          error
	errSingle    error
	errSingleIdx int
}

func (q *fakeQuery) KeysOnly() Query {
	return q
}

func (q *fakeQuery) Limit(i int) Query {
	q.limit = i
	return q
}

func (q *fakeQuery) FailAll() Query {
	q.err = errors.New("Query fail all")
	return q
}

func (q *fakeQuery) Fail(i int) Query {
	q.errSingleIdx = i
	q.errSingle = errors.New("Query fail")
	return q
}

type fakeDatastore struct {
	RawInterface
	aid string
	ns  string
}

func (f *fakeDatastore) NewKey(kind, stringID string, intID int64, parent Key) Key {
	id := interface{}(stringID)
	if stringID == "" {
		id = intID
	}
	return mkKey(f.aid, f.ns, kind, id, parent)
}

func (f *fakeDatastore) NewQuery(string) Query {
	return &fakeQuery{}
}

func (f *fakeDatastore) Run(q Query, cb RawRunCB) error {
	rq := q.(*fakeQuery)
	if rq.err != nil {
		return rq.err
	}
	for i := 0; i < rq.limit; i++ {
		if rq.errSingle != nil && i == rq.errSingleIdx {
			return rq.errSingle
		} else {
			k := f.NewKey("Kind", "", int64(i+1), nil)
			if i == 10 {
				k = f.NewKey("Kind", "eleven", 0, nil)
			}
			pm := PropertyMap{"Value": {MkProperty(i)}}
			if !cb(k, pm, nil) {
				break
			}
		}
	}
	return nil
}

func (f *fakeDatastore) PutMulti(keys []Key, vals []PropertyMap, cb PutMultiCB) error {
	if keys[0].Kind() == "FailAll" {
		return errors.New("PutMulti fail all")
	}
	assertExtra := false
	if _, err := vals[0].GetMeta("assertExtra"); err == nil {
		assertExtra = true
	}
	for i, k := range keys {
		err := error(nil)
		if k.Kind() == "Fail" {
			err = errors.New("PutMulti fail")
		} else {
			So(vals[i]["Value"], ShouldResemble, []Property{MkProperty(i)})
			if assertExtra {
				So(vals[i]["Extra"], ShouldResemble, []Property{MkProperty("whoa")})
			}
			if k.Incomplete() {
				k = mkKey(k.AppID(), k.Namespace(), k.Kind(), int64(i+1), k.Parent())
			}
		}
		cb(k, err)
	}
	return nil
}

func (f *fakeDatastore) GetMulti(keys []Key, _meta MultiMetaGetter, cb GetMultiCB) error {
	if keys[0].Kind() == "FailAll" {
		return errors.New("GetMulti fail all")
	}
	for i, k := range keys {
		if k.Kind() == "Fail" {
			cb(nil, errors.New("GetMulti fail"))
		} else {
			cb(PropertyMap{"Value": {MkProperty(i + 1)}}, nil)
		}
	}
	return nil
}

func (f *fakeDatastore) DeleteMulti(keys []Key, cb DeleteMultiCB) error {
	if keys[0].Kind() == "FailAll" {
		return errors.New("DeleteMulti fail all")
	}
	for _, k := range keys {
		if k.Kind() == "Fail" {
			cb(errors.New("DeleteMulti fail"))
		} else {
			cb(nil)
		}
	}
	return nil
}

type badStruct struct {
	ID    int64     `gae:"$id"`
	Compy complex64 // bad type
}

type CommonStruct struct {
	ID     int64 `gae:"$id"`
	Parent Key   `gae:"$parent"`

	Value int64
}

type permaBad struct {
	PropertyLoadSaver
}

func (f *permaBad) Load(pm PropertyMap) error {
	return errors.New("permaBad")
}

type FakePLS struct {
	IntID    int64
	StringID string
	Kind     string

	Value     int64
	gotLoaded bool

	failGetMeta bool
	failLoad    bool
	failProblem bool
	failSave    bool
	failSetMeta bool
}

var _ PropertyLoadSaver = (*FakePLS)(nil)

func (f *FakePLS) Load(pm PropertyMap) error {
	if f.failLoad {
		return errors.New("FakePLS.Load")
	}
	f.gotLoaded = true
	f.Value = pm["Value"][0].Value().(int64)
	return nil
}

func (f *FakePLS) Save(withMeta bool) (PropertyMap, error) {
	if f.failSave {
		return nil, errors.New("FakePLS.Save")
	}
	ret := PropertyMap{
		"Value": {MkProperty(f.Value)},
		"Extra": {MkProperty("whoa")},
	}
	if withMeta {
		id, _ := f.GetMeta("id")
		ret.SetMeta("id", id)
		if f.Kind == "" {
			ret.SetMeta("kind", "FakePLS")
		} else {
			ret.SetMeta("kind", f.Kind)
		}
		ret.SetMeta("assertExtra", true)
	}
	return ret, nil
}

func (f *FakePLS) GetMetaDefault(key string, dflt interface{}) interface{} {
	return GetMetaDefaultImpl(f.GetMeta, key, dflt)
}

func (f *FakePLS) GetMeta(key string) (interface{}, error) {
	if f.failGetMeta {
		return nil, errors.New("FakePLS.GetMeta")
	}
	switch key {
	case "id":
		if f.StringID != "" {
			return f.StringID, nil
		}
		return f.IntID, nil
	case "kind":
		if f.Kind == "" {
			return "FakePLS", nil
		}
		return f.Kind, nil
	}
	return nil, ErrMetaFieldUnset
}

func (f *FakePLS) SetMeta(key string, val interface{}) error {
	if f.failSetMeta {
		return errors.New("FakePL.SetMeta")
	}
	if key == "id" {
		switch x := val.(type) {
		case int64:
			f.IntID = x
		case string:
			f.StringID = x
		}
		return nil
	}
	if key == "kind" {
		f.Kind = val.(string)
		return nil
	}
	return ErrMetaFieldUnset
}

func (f *FakePLS) Problem() error {
	if f.failProblem {
		return errors.New("FakePLS.Problem")
	}
	return nil
}

func TestKeyForObj(t *testing.T) {
	t.Parallel()

	Convey("Test interface.KeyForObj", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)

		k := ds.NewKey("Hello", "world", 0, nil)

		Convey("good", func() {
			Convey("struct containing $key", func() {
				type keyStruct struct {
					Key Key `gae:"$key"`
				}

				ks := &keyStruct{k}
				So(ds.KeyForObj(ks), ShouldEqual, k)
			})

			Convey("struct containing default $id and $kind", func() {
				type idStruct struct {
					id  string `gae:"$id,wut"`
					knd string `gae:"$kind,SuperKind"`
				}

				So(ds.KeyForObj(&idStruct{}).String(), ShouldEqual, `/SuperKind,wut`)
			})

			Convey("struct containing $id and $parent", func() {
				So(ds.KeyForObj(&CommonStruct{ID: 4}).String(), ShouldEqual, `/CommonStruct,4`)

				So(ds.KeyForObj(&CommonStruct{ID: 4, Parent: k}).String(), ShouldEqual, `/Hello,world/CommonStruct,4`)
			})

			Convey("a propmap with $key", func() {
				pm := PropertyMap{}
				pm.SetMeta("key", k)
				So(ds.KeyForObj(pm).String(), ShouldEqual, `/Hello,world`)
			})

			Convey("a propmap with $id, $kind, $parent", func() {
				pm := PropertyMap{}
				pm.SetMeta("id", 100)
				pm.SetMeta("kind", "Sup")
				So(ds.KeyForObj(pm).String(), ShouldEqual, `/Sup,100`)

				pm.SetMeta("parent", k)
				So(ds.KeyForObj(pm).String(), ShouldEqual, `/Hello,world/Sup,100`)
			})

			Convey("a pls with $id, $parent", func() {
				pls := GetPLS(&CommonStruct{ID: 1})
				So(ds.KeyForObj(pls).String(), ShouldEqual, `/CommonStruct,1`)

				pls.SetMeta("parent", k)
				So(ds.KeyForObj(pls).String(), ShouldEqual, `/Hello,world/CommonStruct,1`)
			})

		})

		Convey("bad", func() {
			Convey("a propmap without $kind", func() {
				pm := PropertyMap{}
				pm.SetMeta("id", 100)
				So(func() { ds.KeyForObj(pm) }, ShouldPanic)
			})
		})
	})
}

func TestPut(t *testing.T) {
	t.Parallel()

	Convey("Test Put/PutMulti", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)

		Convey("bad", func() {
			Convey("static can't serialize", func() {
				bss := []badStruct{{}, {}}
				So(ds.PutMulti(bss).Error(), ShouldContainSubstring, "invalid PutMulti input")
			})

			Convey("static ptr can't serialize", func() {
				bss := []*badStruct{{}, {}}
				So(ds.PutMulti(bss).Error(), ShouldContainSubstring, "invalid PutMulti input")
			})

			Convey("static bad type (non-slice)", func() {
				So(ds.PutMulti(100).Error(), ShouldContainSubstring, "invalid PutMulti input")
			})

			Convey("static bad type (slice of bad type)", func() {
				So(ds.PutMulti([]int{}).Error(), ShouldContainSubstring, "invalid PutMulti input")
			})

			Convey("dynamic can't serialize", func() {
				fplss := []FakePLS{{failSave: true}, {}}
				So(ds.PutMulti(fplss).Error(), ShouldContainSubstring, "FakePLS.Save")
			})

			Convey("can't get keys", func() {
				fplss := []FakePLS{{failGetMeta: true}, {}}
				So(ds.PutMulti(fplss).Error(), ShouldContainSubstring, "unable to extract $kind")
			})

			Convey("get single error for RPC failure", func() {
				fplss := []FakePLS{{Kind: "FailAll"}, {}}
				So(ds.PutMulti(fplss).Error(), ShouldEqual, "PutMulti fail all")
			})

			Convey("get multi error for individual failures", func() {
				fplss := []FakePLS{{}, {Kind: "Fail"}}
				So(ds.PutMulti(fplss), ShouldResemble, errors.MultiError{nil, errors.New("PutMulti fail")})
			})

			Convey("put with non-modifyable type is an error", func() {
				cs := CommonStruct{}
				So(ds.Put(cs).Error(), ShouldContainSubstring, "invalid Put input type")
			})
		})

		Convey("ok", func() {
			Convey("[]S", func() {
				css := make([]CommonStruct, 7)
				for i := range css {
					if i == 4 {
						css[i].ID = 200
					}
					css[i].Value = int64(i)
				}
				So(ds.PutMulti(css), ShouldBeNil)
				for i, cs := range css {
					expect := int64(i + 1)
					if i == 4 {
						expect = 200
					}
					So(cs.ID, ShouldEqual, expect)
				}
			})

			Convey("[]*S", func() {
				css := make([]*CommonStruct, 7)
				for i := range css {
					css[i] = &CommonStruct{Value: int64(i)}
					if i == 4 {
						css[i].ID = 200
					}
				}
				So(ds.PutMulti(css), ShouldBeNil)
				for i, cs := range css {
					expect := int64(i + 1)
					if i == 4 {
						expect = 200
					}
					So(cs.ID, ShouldEqual, expect)
				}

				s := &CommonStruct{}
				So(ds.Put(s), ShouldBeNil)
				So(s.ID, ShouldEqual, 1)
			})

			Convey("[]P", func() {
				fplss := make([]FakePLS, 7)
				for i := range fplss {
					fplss[i].Value = int64(i)
					if i == 4 {
						fplss[i].IntID = int64(200)
					}
				}
				So(ds.PutMulti(fplss), ShouldBeNil)
				for i, fpls := range fplss {
					expect := int64(i + 1)
					if i == 4 {
						expect = 200
					}
					So(fpls.IntID, ShouldEqual, expect)
				}

				pm := PropertyMap{"Value": {MkProperty(0)}, "$kind": {MkPropertyNI("Pmap")}}
				So(ds.Put(pm), ShouldBeNil)
				So(ds.KeyForObj(pm).IntID(), ShouldEqual, 1)
			})

			Convey("[]P (map)", func() {
				pms := make([]PropertyMap, 7)
				for i := range pms {
					pms[i] = PropertyMap{
						"$kind": {MkProperty("Pmap")},
						"Value": {MkProperty(i)},
					}
					if i == 4 {
						pms[i].SetMeta("id", int64(200))
					}
				}
				So(ds.PutMulti(pms), ShouldBeNil)
				for i, pm := range pms {
					expect := int64(i + 1)
					if i == 4 {
						expect = 200
					}
					So(ds.KeyForObj(pm).String(), ShouldEqual, fmt.Sprintf("/Pmap,%d", expect))
				}
			})

			Convey("[]*P", func() {
				fplss := make([]*FakePLS, 7)
				for i := range fplss {
					fplss[i] = &FakePLS{Value: int64(i)}
					if i == 4 {
						fplss[i].IntID = int64(200)
					}
				}
				So(ds.PutMulti(fplss), ShouldBeNil)
				for i, fpls := range fplss {
					expect := int64(i + 1)
					if i == 4 {
						expect = 200
					}
					So(fpls.IntID, ShouldEqual, expect)
				}
			})

			Convey("[]*P (map)", func() {
				pms := make([]*PropertyMap, 7)
				for i := range pms {
					pms[i] = &PropertyMap{
						"$kind": {MkProperty("Pmap")},
						"Value": {MkProperty(i)},
					}
					if i == 4 {
						pms[i].SetMeta("id", int64(200))
					}
				}
				So(ds.PutMulti(pms), ShouldBeNil)
				for i, pm := range pms {
					expect := int64(i + 1)
					if i == 4 {
						expect = 200
					}
					So(ds.KeyForObj(*pm).String(), ShouldEqual, fmt.Sprintf("/Pmap,%d", expect))
				}
			})

			Convey("[]I", func() {
				ifs := []interface{}{
					&CommonStruct{Value: 0},
					&FakePLS{Value: 1},
					PropertyMap{"Value": {MkProperty(2)}, "$kind": {MkPropertyNI("Pmap")}},
					&PropertyMap{"Value": {MkProperty(3)}, "$kind": {MkPropertyNI("Pmap")}},
				}
				So(ds.PutMulti(ifs), ShouldBeNil)
				for i := range ifs {
					switch i {
					case 0:
						So(ifs[i].(*CommonStruct).ID, ShouldEqual, 1)
					case 1:
						fpls := ifs[i].(*FakePLS)
						So(fpls.IntID, ShouldEqual, 2)
					case 2:
						So(ds.KeyForObj(ifs[i].(PropertyMap)).String(), ShouldEqual, "/Pmap,3")
					case 3:
						So(ds.KeyForObj(*ifs[i].(*PropertyMap)).String(), ShouldEqual, "/Pmap,4")
					}
				}
			})

		})

	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	Convey("Test Delete/DeleteMulti", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)
		So(ds, ShouldNotBeNil)

		Convey("bad", func() {
			Convey("get single error for RPC failure", func() {
				keys := []Key{
					mkKey("s~aid", "ns", "FailAll", 1, nil),
					mkKey("s~aid", "ns", "Ok", 1, nil),
				}
				So(ds.DeleteMulti(keys).Error(), ShouldEqual, "DeleteMulti fail all")
			})

			Convey("get multi error for individual failure", func() {
				keys := []Key{
					ds.NewKey("Ok", "", 1, nil),
					ds.NewKey("Fail", "", 2, nil),
				}
				So(ds.DeleteMulti(keys).Error(), ShouldEqual, "DeleteMulti fail")
			})

			Convey("get single error when deleting a single", func() {
				k := ds.NewKey("Fail", "", 1, nil)
				So(ds.Delete(k).Error(), ShouldEqual, "DeleteMulti fail")
			})
		})

	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	Convey("Test Get/GetMulti", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)
		So(ds, ShouldNotBeNil)

		Convey("bad", func() {
			Convey("static can't serialize", func() {
				toGet := []badStruct{{}, {}}
				So(ds.GetMulti(toGet).Error(), ShouldContainSubstring, "invalid GetMulti input")
			})

			Convey("can't get keys", func() {
				fplss := []FakePLS{{failGetMeta: true}, {}}
				So(ds.GetMulti(fplss).Error(), ShouldContainSubstring, "unable to extract $kind")
			})

			Convey("get single error for RPC failure", func() {
				fplss := []FakePLS{
					{IntID: 1, Kind: "FailAll"},
					{IntID: 2},
				}
				So(ds.GetMulti(fplss).Error(), ShouldEqual, "GetMulti fail all")
			})

			Convey("get multi error for individual failures", func() {
				fplss := []FakePLS{{IntID: 1}, {IntID: 2, Kind: "Fail"}}
				So(ds.GetMulti(fplss), ShouldResemble, errors.MultiError{nil, errors.New("GetMulti fail")})
			})

			Convey("get with non-modifiable type is an error", func() {
				cs := CommonStruct{}
				So(ds.Get(cs).Error(), ShouldContainSubstring, "invalid Get input type")
			})

			Convey("failure to save metadata is an issue too", func() {
				cs := &FakePLS{failSave: true}
				So(ds.Get(cs).Error(), ShouldContainSubstring, "FakePLS.Save")
			})
		})

		Convey("ok", func() {
			Convey("Get", func() {
				cs := &CommonStruct{ID: 1}
				So(ds.Get(cs), ShouldBeNil)
				So(cs.Value, ShouldEqual, 1)
			})

			Convey("Raw access too", func() {
				rds := ds.Raw()
				keys := []Key{rds.NewKey("Kind", "", 1, nil)}
				So(rds.GetMulti(keys, nil, func(pm PropertyMap, err error) {
					So(err, ShouldBeNil)
					So(pm["Value"][0].Value(), ShouldEqual, 1)
				}), ShouldBeNil)
			})
		})

	})
}

func TestGetAll(t *testing.T) {
	t.Parallel()

	Convey("Test GetAll", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)
		So(ds, ShouldNotBeNil)

		q := ds.NewQuery("").Limit(5)

		Convey("bad", func() {
			Convey("nil target", func() {
				So(ds.GetAll(q, (*[]PropertyMap)(nil)).Error(), ShouldContainSubstring, "dst: <nil>")
			})

			Convey("bad type", func() {
				output := 100
				So(ds.GetAll(q, &output).Error(), ShouldContainSubstring, "invalid GetAll input type")
			})

			Convey("bad type (non pointer)", func() {
				So(ds.GetAll(q, "moo").Error(), ShouldContainSubstring, "must have a ptr-to-slice")
			})

			Convey("bad type (underspecified)", func() {
				output := []PropertyLoadSaver(nil)
				So(ds.GetAll(q, &output).Error(), ShouldContainSubstring, "invalid GetAll input type")
			})
		})

		Convey("ok", func() {
			Convey("*[]S", func() {
				output := []CommonStruct(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.ID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]*S", func() {
				output := []*CommonStruct(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.ID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]P", func() {
				output := []FakePLS(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.gotLoaded, ShouldBeTrue)
					So(o.IntID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]P (map)", func() {
				output := []PropertyMap(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					k, err := o.GetMeta("key")
					So(err, ShouldBeNil)
					So(k.(Key).IntID(), ShouldEqual, i+1)
					So(o["Value"][0].Value().(int64), ShouldEqual, i)
				}
			})

			Convey("*[]*P", func() {
				output := []*FakePLS(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.gotLoaded, ShouldBeTrue)
					So(o.IntID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]*P (map)", func() {
				output := []*PropertyMap(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, op := range output {
					o := *op
					k, err := o.GetMeta("key")
					So(err, ShouldBeNil)
					So(k.(Key).IntID(), ShouldEqual, i+1)
					So(o["Value"][0].Value().(int64), ShouldEqual, i)
				}
			})

			Convey("*[]Key", func() {
				output := []Key(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, k := range output {
					So(k.IntID(), ShouldEqual, i+1)
				}
			})

		})
	})
}

func TestRun(t *testing.T) {
	t.Parallel()

	Convey("Test Run", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)
		So(ds, ShouldNotBeNil)

		q := ds.NewQuery("").Limit(5)

		Convey("bad", func() {
			assertBadTypePanics := func(cb interface{}) {
				defer func() {
					err, _ := recover().(error)
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring,
						"cb does not match the required callback signature")
				}()
				ds.Run(q, cb)
			}

			Convey("not a function", func() {
				assertBadTypePanics("I am a potato")
			})

			Convey("bad proto type", func() {
				assertBadTypePanics(func(v int, _ CursorCB) bool {
					panic("never here!")
				})
			})

			Convey("wrong # args", func() {
				assertBadTypePanics(func(v CommonStruct, _ CursorCB) {
					panic("never here!")
				})
			})

			Convey("wrong ret type", func() {
				assertBadTypePanics(func(v CommonStruct, _ CursorCB) error {
					panic("never here!")
				})
			})

			Convey("bad 2nd arg", func() {
				assertBadTypePanics(func(v CommonStruct, _ Cursor) bool {
					panic("never here!")
				})
			})

			Convey("early abort on error", func() {
				rq := q.(*fakeQuery).Fail(3)
				i := 0
				So(ds.Run(rq, func(c CommonStruct, _ CursorCB) bool {
					i++
					return true
				}).Error(), ShouldEqual, "Query fail")
				So(i, ShouldEqual, 3)
			})

			Convey("return error on serialization failure", func() {
				So(ds.Run(q, func(_ permaBad, _ CursorCB) bool {
					panic("never here")
				}).Error(), ShouldEqual, "permaBad")
			})
		})

		Convey("ok", func() {
			Convey("*S", func() {
				i := 0
				So(ds.Run(q, func(cs *CommonStruct, _ CursorCB) bool {
					So(cs.ID, ShouldEqual, i+1)
					So(cs.Value, ShouldEqual, i)
					i++
					return true
				}), ShouldBeNil)
			})

			Convey("*P", func() {
				i := 0
				So(ds.Run(q.Limit(12), func(fpls *FakePLS, _ CursorCB) bool {
					So(fpls.gotLoaded, ShouldBeTrue)
					if i == 10 {
						So(fpls.StringID, ShouldEqual, "eleven")
					} else {
						So(fpls.IntID, ShouldEqual, i+1)
					}
					So(fpls.Value, ShouldEqual, i)
					i++
					return true
				}), ShouldBeNil)
			})

			Convey("*P (map)", func() {
				i := 0
				So(ds.Run(q, func(pm *PropertyMap, _ CursorCB) bool {
					k, err := pm.GetMeta("key")
					So(err, ShouldBeNil)
					So(k.(Key).IntID(), ShouldEqual, i+1)
					So((*pm)["Value"][0].Value(), ShouldEqual, i)
					i++
					return true
				}), ShouldBeNil)
			})

			Convey("S", func() {
				i := 0
				So(ds.Run(q, func(cs CommonStruct, _ CursorCB) bool {
					So(cs.ID, ShouldEqual, i+1)
					So(cs.Value, ShouldEqual, i)
					i++
					return true
				}), ShouldBeNil)
			})

			Convey("P", func() {
				i := 0
				So(ds.Run(q, func(fpls FakePLS, _ CursorCB) bool {
					So(fpls.gotLoaded, ShouldBeTrue)
					So(fpls.IntID, ShouldEqual, i+1)
					So(fpls.Value, ShouldEqual, i)
					i++
					return true
				}), ShouldBeNil)
			})

			Convey("P (map)", func() {
				i := 0
				So(ds.Run(q, func(pm PropertyMap, _ CursorCB) bool {
					k, err := pm.GetMeta("key")
					So(err, ShouldBeNil)
					So(k.(Key).IntID(), ShouldEqual, i+1)
					So(pm["Value"][0].Value(), ShouldEqual, i)
					i++
					return true
				}), ShouldBeNil)
			})

			Convey("Key", func() {
				i := 0
				So(ds.Run(q, func(k Key, _ CursorCB) bool {
					So(k.IntID(), ShouldEqual, i+1)
					i++
					return true
				}), ShouldBeNil)
			})

		})
	})
}