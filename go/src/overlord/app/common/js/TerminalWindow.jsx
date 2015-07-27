// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// Terminal Window wiget
//
// props:
//   id: DOM id
//   path: path to websocket
//   title: window title
//   onOpen: callback for connection open
//   onClose: callback for connection close
//   onError: callback for connection error
//   onMessage: callback for message
//   onCloseClicked: callback for close button clicked


var TerminalWindow = React.createClass({
  componentDidMount: function () {
    var el = document.getElementById(this.props.id);
    var url = "ws" + ((window.location.protocol == "https:")? "s": "" ) +
              "://" + window.location.host + this.props.path;
    var sock = new WebSocket(url);

    var $el = $(el);

    this.sock = sock;
    this.el = el;

    $el.draggable({
      // Once the window is dragged, make it position fixed.
      stop: function () {
        offsets = el.getBoundingClientRect();
        $el.css({
          position: 'fixed',
          top: offsets.top+"px",
          left: offsets.left+"px"
        });
      },
      cancel: ".terminal"
    });

    sock.onerror = function (e) {
      var callback = this.props.onError;
      if (typeof(callback) != "undefined") {
        (callback.bind(this))(e);
      }
    }.bind(this);

    sock.onopen = function (e) {
      var term = new Terminal({
        cols: 80,
        rows: 24,
        useStyle: true,
        screenKeys: true
      });

      term.open(el);

      term.on('title', function (title) {
        $el.find('.terminal-title').text(title);
      });

      term.on('data', function (data) {
        sock.send(data);
      });

      sock.onmessage = function (msg) {
        var data = Base64.decode(msg.data);
        term.write(data);

        var callback = this.props.onMessage;
        if (typeof(callback) != "undefined") {
          (callback.bind(this))(msg);
        }
      }.bind(this);

      sock.onclose = function (e) {
        var callback = this.props.onClose;
        if (typeof(callback) != "undefined") {
          (callback.bind(this))(e);
        }
      }.bind(this);

      var callback = this.props.onOpen;
      if (typeof(callback) != "undefined") {
        (callback.bind(this))(e);
      }
    }.bind(this);
  },
  onWindowMouseDown: function (e) {
    if (typeof(window.maxz) == "undefined") {
      window.maxz = 100;
    }
    var $el = $(this.el);
    if ($el.css("z-index") != window.maxz) {
      window.maxz += 1;
      $el.css("z-index", window.maxz);
    }
  },
  onCloseMouseUp: function (e) {
    var callback = this.props.onCloseClicked;
    if (typeof(callback) != "undefined") {
      (callback.bind(this))(e);
    }
    this.sock.close();
  },
  render: function () {
    return (
      <div className="terminal-window" id={this.props.id}
          onMouseDown={this.onWindowMouseDown}>
        <div className="terminal-title">{this.props.title}</div>
        <div className="terminal-control">
          <div className="terminal-close" onMouseUp={this.onCloseMouseUp}></div>
        </div>
      </div>
    );
  }
});

