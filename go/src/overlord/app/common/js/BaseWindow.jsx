// Copyright 2016 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// Defines common functions of Windows.

var BaseWindow = {
  makeDraggable: function (cancel) {
    var el = this.refs.window;
    var $el = $(el);
    $el.draggable({
      // Once the window is dragged, make its position fixed.
      stop: function () {
        offsets = el.getBoundingClientRect();
        $el.css({
          position: "fixed",
          top: offsets.top+"px",
          left: offsets.left+"px"
        });
      },
      cancel: cancel
    });
  },
  enableDraggable: function () {
    $(this.refs.window).draggable('enable');
  },
  disableDraggable: function () {
    $(this.refs.window).draggable('disable');
  },
  onWindowMouseDown: function (event) {
    this.bringToFront();
  },
  onCloseMouseUp: function (event) {
    var callback = this.props.onCloseClicked;
    if (typeof(callback) != "undefined") {
      (callback.bind(this))(event);
    }
  },
  onMinimizeMouseUp: function (event) {
    var callback = this.props.onMinimizeClicked;
    if (typeof(callback) != "undefined") {
      (callback.bind(this))(event);
    }
  },
  bringToFront: function () {
    if (typeof(window.maxz) == "undefined") {
      window.maxz = parseInt($('.navbar').css('z-index')) + 1;
    }
    var $el = $(this.refs.window);
    if ($el.css("z-index") != window.maxz) {
      window.maxz += 1;
      $el.css("z-index", window.maxz);
    }
  }
};
