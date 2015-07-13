// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// Terminal Window widget
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
          position: "fixed",
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

      term.on("title", function (title) {
        $el.find(".terminal-title").text(title);
      });

      term.on("data", function (data) {
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

      // Calculate terminal and terminal-window width/height relation.
      // Used for resize procedure
      // we will hide right and bottom border of teriminal.
      // and add the same size to terminal-window for good looking and resize indicator
      var $terminal = $el.find(".terminal");
      var termBorderRightWidth = $terminal.css("border-right-width");
      var termBorderBottomWidth = $terminal.css("border-bottom-width");
      var termWidthOffset = $el.outerWidth() - term.element.clientWidth;
      var termHeightOffset = $el.outerHeight() - term.element.clientHeight;
      var totalWidthOffset = termWidthOffset + parseInt(termBorderRightWidth);
      var totalHeightOffset = termHeightOffset + parseInt(termBorderBottomWidth);

      // hide terminal right and bottom border
      $terminal.css("border-right-width", "0px");
      $terminal.css("border-bottom-width", "0px");

      // initial terminal-window size
      el.style.width = term.element.clientWidth + totalWidthOffset;
      el.style.height = term.element.clientHeight + totalHeightOffset;

      $el.resizable();
      $el.bind("resize", function () {
          // We use CONTROL_START and CONTROL_END to specify the control buffer region.
          // Ghost can use the 2 characters to know the control string.
          // format:
          // CONTROL_START ControlString CONTROL_END
          var CONTROL_START = 128;
          var CONTROL_END = 129;

          // If there is no terminal now, just return.
          // It may happen when we close the window
          if (term.element.clientWidth == 0 || term.element.clientHeight == 0) {
              return;
          }

          // convert to cols/rows
          var widthToColsFactor = term.cols / term.element.clientWidth;
          var heightToRowsFactor = term.rows / term.element.clientHeight;
          newTermWidth = parseInt(el.style.width) - totalWidthOffset;
          newTermHeight = parseInt(el.style.height) - totalHeightOffset;
          newCols = Math.floor(newTermWidth * widthToColsFactor);
          newRows = Math.floor(newTermHeight * heightToRowsFactor);
          if (newCols != term.cols || newRows != term.rows) {
              var msg = {
                  command: "resize",
                  params: [newRows, newCols]
              }
              term.resize(newCols, newRows);
              term.refresh(0, term.rows - 1);

              // Fine tune terminal-window size to match terminal.
              // Prevent white space between terminal-window and terminal.
              el.style.width = term.element.clientWidth + totalWidthOffset;
              el.style.height = term.element.clientHeight + totalHeightOffset;

              // Send to ghost to set new size
              sock.send((new Uint8Array([CONTROL_START])).buffer);
              sock.send(JSON.stringify(msg));
              sock.send((new Uint8Array([CONTROL_END])).buffer);
          }
      });
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

