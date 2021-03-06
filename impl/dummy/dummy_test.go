// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dummy

import (
	"testing"

	dsS "github.com/luci/gae/service/datastore"
	infoS "github.com/luci/gae/service/info"
	mailS "github.com/luci/gae/service/mail"
	mcS "github.com/luci/gae/service/memcache"
	modS "github.com/luci/gae/service/module"
	tqS "github.com/luci/gae/service/taskqueue"
	userS "github.com/luci/gae/service/user"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestContextAccess(t *testing.T) {
	t.Parallel()

	// p is a function which recovers an error and then immediately panics with
	// the contained string. It's defer'd in each test so that we can use the
	// ShouldPanicWith assertion (which does an == comparison and not
	// a reflect.DeepEquals comparison).
	p := func() { panic(recover().(error).Error()) }

	Convey("Context Access", t, func() {
		c := context.Background()

		Convey("blank", func() {
			So(dsS.Raw(c), ShouldBeNil)
			So(mcS.Raw(c), ShouldBeNil)
			So(tqS.Raw(c), ShouldBeNil)
			So(infoS.Raw(c), ShouldBeNil)
		})

		// needed for everything else
		c = infoS.Set(c, Info())

		Convey("Info", func() {
			So(infoS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				infoS.Raw(c).Datacenter()
			}, ShouldPanicWith, "dummy: method Info.Datacenter is not implemented")
		})

		Convey("Datastore", func() {
			c = dsS.SetRaw(c, Datastore())
			So(dsS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				_, _ = dsS.Raw(c).DecodeCursor("wut")
			}, ShouldPanicWith, "dummy: method Datastore.DecodeCursor is not implemented")
		})

		Convey("Memcache", func() {
			c = mcS.SetRaw(c, Memcache())
			So(mcS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				_ = mcS.Add(c, nil)
			}, ShouldPanicWith, "dummy: method Memcache.AddMulti is not implemented")
		})

		Convey("TaskQueue", func() {
			c = tqS.SetRaw(c, TaskQueue())
			So(tqS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				_ = tqS.Purge(c, "")
			}, ShouldPanicWith, "dummy: method TaskQueue.Purge is not implemented")
		})

		Convey("User", func() {
			c = userS.Set(c, User())
			So(userS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				_ = userS.IsAdmin(c)
			}, ShouldPanicWith, "dummy: method User.IsAdmin is not implemented")
		})

		Convey("Mail", func() {
			c = mailS.Set(c, Mail())
			So(mailS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				_ = mailS.Send(c, nil)
			}, ShouldPanicWith, "dummy: method Mail.Send is not implemented")
		})

		Convey("Module", func() {
			c = modS.Set(c, Module())
			So(modS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				modS.List(c)
			}, ShouldPanicWith, "dummy: method Module.List is not implemented")
		})
	})
}
