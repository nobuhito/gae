// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package count

import (
	"fmt"
	"testing"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/mail"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/gae/service/taskqueue"
	"github.com/luci/gae/service/user"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func shouldHaveSuccessesAndErrors(actual interface{}, expected ...interface{}) string {
	a := actual.(Entry)
	if len(expected) != 2 {
		panic("Invalid number of expected, should be 2 (successes, errors).")
	}
	s, e := expected[0].(int), expected[1].(int)

	if val := a.Successes(); val != s {
		return fmt.Sprintf("Actual successes (%d) don't match expected (%d)", val, s)
	}
	if val := a.Errors(); val != e {
		return fmt.Sprintf("Actual errors (%d) don't match expected (%d)", val, e)
	}
	return ""
}

func die(err error) {
	if err != nil {
		panic(err)
	}
}

func TestCount(t *testing.T) {
	t.Parallel()

	Convey("Test Count filter", t, func() {
		c, fb := featureBreaker.FilterRDS(memory.Use(context.Background()), nil)
		c, ctr := FilterRDS(c)

		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		vals := []ds.PropertyMap{{
			"Val":  ds.MkProperty(100),
			"$key": ds.MkPropertyNI(ds.NewKey(c, "Kind", "", 1, nil)),
		}}

		Convey("Calling a ds function should reflect in counter", func() {
			So(ds.Put(c, vals), ShouldBeNil)
			So(ctr.PutMulti.Successes(), ShouldEqual, 1)

			Convey("effects are cumulative", func() {
				So(ds.Put(c, vals), ShouldBeNil)
				So(ctr.PutMulti.Successes(), ShouldEqual, 2)

				Convey("even within transactions", func() {
					die(ds.RunInTransaction(c, func(c context.Context) error {
						So(ds.Put(c, append(vals, vals[0])), ShouldBeNil)
						return nil
					}, nil))
				})
			})
		})

		Convey("errors count against errors", func() {
			fb.BreakFeatures(nil, "GetMulti")

			So(ds.Get(c, vals), ShouldErrLike, `"GetMulti" is broken`)
			So(ctr.GetMulti.Errors(), ShouldEqual, 1)

			fb.UnbreakFeatures("GetMulti")

			So(ds.Put(c, vals), ShouldBeNil)

			die(ds.Get(c, vals))
			So(ctr.GetMulti.Errors(), ShouldEqual, 1)
			So(ctr.GetMulti.Successes(), ShouldEqual, 1)
			So(ctr.GetMulti.Total(), ShouldEqual, 2)
		})

		Convey(`datastore.Stop does not count as an error`, func() {
			fb.BreakFeatures(ds.Stop, "GetMulti")

			So(ds.Get(c, vals), ShouldBeNil)
			So(ctr.GetMulti.Successes(), ShouldEqual, 1)
			So(ctr.GetMulti.Errors(), ShouldEqual, 0)
			So(ctr.GetMulti.Total(), ShouldEqual, 1)
		})
	})

	Convey("works for memcache", t, func() {
		c, ctr := FilterMC(memory.Use(context.Background()))
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		die(memcache.Set(c, memcache.NewItem(c, "hello").SetValue([]byte("sup"))))

		_, err := memcache.GetKey(c, "Wat")
		So(err, ShouldNotBeNil)

		_, err = memcache.GetKey(c, "hello")
		die(err)

		So(ctr.SetMulti, shouldHaveSuccessesAndErrors, 1, 0)
		So(ctr.GetMulti, shouldHaveSuccessesAndErrors, 2, 0)
		So(ctr.NewItem, shouldHaveSuccessesAndErrors, 3, 0)
	})

	Convey("works for taskqueue", t, func() {
		c, ctr := FilterTQ(memory.Use(context.Background()))
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		die(taskqueue.Add(c, "", &taskqueue.Task{Name: "wat"}))
		So(taskqueue.Add(c, "DNE_QUEUE", &taskqueue.Task{Name: "wat"}),
			ShouldErrLike, "UNKNOWN_QUEUE")

		So(ctr.AddMulti, shouldHaveSuccessesAndErrors, 1, 1)
	})

	Convey("works for global info", t, func() {
		c, fb := featureBreaker.FilterGI(memory.Use(context.Background()), nil)
		c, ctr := FilterGI(c)
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		_, err := info.Namespace(c, "foo")
		die(err)
		fb.BreakFeatures(nil, "Namespace")
		_, err = info.Namespace(c, "boom")
		So(err, ShouldErrLike, `"Namespace" is broken`)

		So(ctr.Namespace, shouldHaveSuccessesAndErrors, 1, 1)
	})

	Convey("works for user", t, func() {
		c, fb := featureBreaker.FilterUser(memory.Use(context.Background()), nil)
		c, ctr := FilterUser(c)
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		_, err := user.CurrentOAuth(c, "foo")
		die(err)
		fb.BreakFeatures(nil, "CurrentOAuth")
		_, err = user.CurrentOAuth(c, "foo")
		So(err, ShouldErrLike, `"CurrentOAuth" is broken`)

		So(ctr.CurrentOAuth, shouldHaveSuccessesAndErrors, 1, 1)
	})

	Convey("works for mail", t, func() {
		c, fb := featureBreaker.FilterMail(memory.Use(context.Background()), nil)
		c, ctr := FilterMail(c)
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		err := mail.Send(c, &mail.Message{
			Sender: "admin@example.com",
			To:     []string{"coolDood@example.com"},
			Body:   "hi",
		})
		die(err)

		fb.BreakFeatures(nil, "Send")
		err = mail.Send(c, &mail.Message{
			Sender: "admin@example.com",
			To:     []string{"coolDood@example.com"},
			Body:   "hi",
		})
		So(err, ShouldErrLike, `"Send" is broken`)

		So(ctr.Send, shouldHaveSuccessesAndErrors, 1, 1)
	})
}

func ExampleFilterRDS() {
	// Set up your context using a base service implementation (memory or prod)
	c := memory.Use(context.Background())

	// Apply the counter.FilterRDS
	c, counter := FilterRDS(c)

	// functions use ds from the context like normal... they don't need to know
	// that there are any filters at all.
	someCalledFunc := func(c context.Context) {
		vals := []ds.PropertyMap{{
			"FieldName": ds.MkProperty(100),
			"$key":      ds.MkProperty(ds.NewKey(c, "Kind", "", 1, nil))},
		}
		if err := ds.Put(c, vals); err != nil {
			panic(err)
		}
	}

	// Using the other function.
	someCalledFunc(c)
	someCalledFunc(c)

	// Then we can see what happened!
	fmt.Printf("%d\n", counter.PutMulti.Successes())
	// Output:
	// 2
}
