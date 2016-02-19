// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// External dependencies:
// - term.js: https://github.com/chjj/term.js
//
// Internal dependencies:
// - UploadProgressWidget
//
// View for TerminalWindow:
// - TerminalWindow
//   props:
//     id: DOM id
//     path: path to websocket
//     uploadRequestPath: path to upload the file (without terminal sid)
//     title: window title
//     enableMinimize: a boolean value for enable minimize button
//     enableCopy: a boolean value for enable the copy icon, which allow
//      copying of terminal buffer
//     progressBars: an react reference to UploadProgressWidget instance
//     onOpen: callback for connection open
//     onClose: callback for connection close
//     onError: callback for connection error
//     onMessage: callback for message (binary)
//     onMessage: callback for control message (JSON)
//     onCloseClicked: callback for close button clicked
//     onMinimizeClicked: callback for mininize button clicked

// Terminal control sequence identifier
var CONTROL_START = 128;
var CONTROL_END = 129;


Terminal.prototype.CopyAll = function () {
  var term = this;
  var textarea = term.getCopyTextarea();
  var text = term.grabText(
    0, term.cols - 1,
    0, term.lines.length - 1);
  term.emit("copy", text);
  textarea.focus();
  textarea.textContent = text;
  textarea.value = text;
  textarea.setSelectionRange(0, text.length);
  document.execCommand("Copy");
  setTimeout(function () {
    term.element.focus();
    term.focus();
  }, 1);
};


var TerminalWindow = React.createClass({
  mixins: [BaseWindow],
  getInitialState: function () {
    return {sid: "", maximized: false, window_params: undefined};
  },
  resizeWindowToVisualSize: function (visualWidth, visualHeight) {
    // We use CONTROL_START and CONTROL_END to specify the control buffer
    // region.  Ghost can use the 2 characters to know the control string.
    // format:
    // CONTROL_START ControlString_in_JSON CONTROL_END
    var term = this.term;

    if (visualWidth == 0 || visualHeight == 0) {
      return;
    }

    var widthToColsFactor = term.cols / term.element.clientWidth;
    var heightToRowsFactor = term.rows / term.element.clientHeight;

    var newCols = Math.floor(visualWidth * widthToColsFactor);
    var newRows = Math.floor(visualHeight * heightToRowsFactor);

    // Change visual size
    term.element.width = visualWidth;
    term.element.height = visualHeight;

    if (newCols != term.cols || newRows != term.rows) {
      var msg = {
        command: "resize",
        params: [newRows, newCols]
      }
      term.resize(newCols, newRows);
      term.refresh(0, term.rows - 1);

      // Send terminal control sequence
      this.sock.send((new Uint8Array([CONTROL_START])).buffer);
      this.sock.send(JSON.stringify(msg));
      this.sock.send((new Uint8Array([CONTROL_END])).buffer);
    }
  },
  componentDidMount: function () {
    var el = this.refs.window;
    var url = "ws" + ((window.location.protocol == "https:")? "s": "" ) +
              "://" + window.location.host + this.props.path;
    var sock = new WebSocket(url);

    var $el = $(el);

    this.sock = sock;

    this.makeDraggable(".terminal");
    this.bringToFront();

    sock.onerror = function (event) {
      var callback = this.props.onError;
      if (typeof(callback) != "undefined") {
        (callback.bind(this))(event);
      }
    }.bind(this);

    var term = new Terminal({
      cols: 80,
      rows: 24,
      scrollback: 10000,
      useStyle: true,
      screenKeys: false
    });
    this.term = term;

    var bindDisconnectEvent = function () {
      var overlay = $el.find(".terminal-disconnected-overlay");
      overlay.on("click", function (event) {
        overlay.css("display", "none");
      })
    }

    var bindDragAndDropEvents = function () {
      var termDom = $el.find(".terminal");
      var overlay = $el.find(".terminal-drop-overlay");

      termDom.on("dragenter", function (event) {
        event.preventDefault();
        event.stopPropagation();
        overlay.css("display", "block");
      }.bind(this));

      overlay.on("dragenter", function (event) {
        event.preventDefault();
        event.stopPropagation();
      });

      overlay.on("dragover", function (event) {
        event.preventDefault();
        event.stopPropagation();
      });

      overlay.on("dragleave", function (event) {
        event.preventDefault();
        event.stopPropagation();
        overlay.css("display", "none");
      });

      overlay.on("drop", function (event) {
        event.preventDefault();
        event.stopPropagation();
        var files = event.originalEvent.dataTransfer.files;

        for (var i = 0; i < files.length; i++) {
          this.props.progressBars.upload(this.props.uploadRequestPath, files[i],
                                         undefined, this.state.sid);
        }
        $el.find(".terminal-drop-overlay").css("display", "none");
      }.bind(this));
    }.bind(this);

    sock.onopen = function (event) {
      term.open(el);
      term.on("title", function (title) {
        $el.find(".app-window-title").text(title);
      });

      term.on("data", function (data) {
        if (sock.readyState == 1) { // OPEN
          sock.send(data);
        }
      });

      sock.onmessage = function (msg) {
        if (msg.data instanceof Blob) {
          var callback = this.props.onMessage;
          ReadBlobAsText(msg.data, function (text) {
            term.write(text);
            if (typeof(callback) != "undefined") {
              (callback.bind(this))(text);
            }
          }.bind(this));
        // In Javacscript, a string literal is not a instance of String.
        // We check both cases here.
        } else if (msg.data instanceof String || typeof(msg.data) == "string") {
          var control = JSON.parse(msg.data);
          if (control.type == "sid") {
            this.setState({sid: control.data})
          }
          var callback = this.props.onControl;
          if (typeof(callback) != "undefined") {
            (callback.bind(this))(control);
          }
        }
      }.bind(this);

      sock.onclose = function (event) {
        term.write("\r\nConnection lost.");
        $el.find(".terminal-disconnected-overlay").css("display", "block");

        var callback = this.props.onClose;
        if (typeof(callback) != "undefined") {
          (callback.bind(this))(event);
        }
      }.bind(this);

      // Bind events
      bindDisconnectEvent();

      // Only bind drag and drop event if uploadRequestPath is provided
      if (typeof(this.props.uploadRequestPath) != "undefined") {
        bindDragAndDropEvents();
      }

      var callback = this.props.onOpen;
      if (typeof(callback) != "undefined") {
        (callback.bind(this))(event);
      }
    }.bind(this);
  },
  onCloseMouseUp2: function (event) {
    this.onCloseMouseUp();
    this.sock.close();
  },
  onMaximizeMouseUp: function (event) {
    var el = this.refs.window;
    if (!this.state.maximized) {
      var window_params = {
        left: el.offsetLeft,
        top: el.offsetTop,
        width: this.term.element.clientWidth,
        height: this.term.element.clientHeight,
      };
      var offsetWidth = el.offsetWidth - this.term.element.clientWidth;
      var offsetHeight = el.offsetHeight - this.term.element.clientHeight;
      this.disableDraggable();
      this.resizeWindowToVisualSize(window.innerWidth - offsetWidth,
                                    window.innerHeight - offsetHeight);
      this.setState(function (state, props) {
        state.maximized = true;
        state.window_params = window_params;
      });
    } else {
      var params = this.state.window_params;
      this.enableDraggable();
      $(el).css({
        top: params.top,
        left: params.left,
        position: "fixed",
      });
      this.resizeWindowToVisualSize(params.width, params.height);
      this.setState(function (state, props) {
        state.maximized = false;
        state.window_params = undefined;
      });
    }
  },
  onCopyMouseUp: function (event) {
    this.term.CopyAll();
  },
  render: function () {
    var minimize_icon_node = "",
        maximize_icon_node = "",
        copy_icon_node = "",
        app_window_class = "";
    if (this.props.enableCopy) {
      copy_icon_node = (
          <div className="app-window-icon app-window-copy"
           onMouseUp={this.onCopyMouseUp}></div>
      );
    }
    if (this.props.enableMinimize) {
      minimize_icon_node = (
          <div className="app-window-icon app-window-minimize"
           onMouseUp={this.onMinimizeMouseUp}></div>
      );
    }
    if (this.props.enableMaximize) {
      maximize_icon_node = (
          <div className="app-window-icon app-window-maximize"
           onMouseUp={this.onMaximizeMouseUp}></div>
      );
    }
    if (this.state.maximized) {
      app_window_class = "app-window-maximized";
    }
    return (
      <div className={"app-window " + app_window_class} ref="window"
          onMouseDown={this.onWindowMouseDown}>
        <div className="app-window-title">{this.props.title}</div>
        <div className="app-window-control">
          {copy_icon_node}
          {minimize_icon_node}
          {maximize_icon_node}
          <div className="app-window-icon app-window-close"
           onMouseUp={this.onCloseMouseUp2}></div>
        </div>
        <div className="terminal-overlay terminal-drop-overlay">
          Drop files here to upload
        </div>
        <div className="terminal-overlay terminal-disconnected-overlay">
          Connection lost
        </div>
      </div>
    );
  }
});
