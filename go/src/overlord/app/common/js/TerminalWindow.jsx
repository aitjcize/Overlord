// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// External dependencies:
// - term.js: https://github.com/chjj/term.js
//
// View for TerminalWindow:
// - TerminalWindow
//   props:
//     id: DOM id
//     path: path to websocket
//     uploadPath: path to upload the file (without terminal sid)
//     title: window title
//     enableMinimize: a boolean value for enable minimize button
//     enableCopy: a boolean value for enable the copy icon, which allow
//      copying of terminal buffer
//     onOpen: callback for connection open
//     onClose: callback for connection close
//     onError: callback for connection error
//     onMessage: callback for message (binary)
//     onMessage: callback for control message (JSON)
//     onCloseClicked: callback for close button clicked
//     onMinimizeClicked: callback for mininize button clicked
//
// - UploadProgress
//   - [(ProgressBar|ProgressBar) ...]

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
    return {sid: ""};
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
      useStyle: true,
      screenKeys: true
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

        var $this = this;
        var files = event.originalEvent.dataTransfer.files;

        for (var i = 0; i < files.length; i++) {
          function postFile(file) {
            var id = randomID();
            var formData = new FormData();
            formData.append("file", file);

            $this.props.progressBars.addRecord({filename: file.name, id: id});
            $.ajax({
              xhr: function () {
                var xhr = new window.XMLHttpRequest();
                xhr.upload.addEventListener("progress", function (event) {
                  if (event.lengthComputable) {
                    var percentComplete = Math.round(event.loaded * 100 /
                                                     event.total);
                    $("#" + id).css("width", percentComplete + "%");
                    $("#" + id + " > .percent").text(percentComplete + "%");
                  }
                }, false);
                return xhr;
              },
              url: $this.props.uploadPath + "?terminal_sid=" + $this.state.sid,
              data: formData,
              cache: false,
              contentType: false,
              processData: false,
              type: "POST",
              success: function (data) {
                $("#" + id).css("width", "100%");
                // Display the progressbar for 1 more seconds after complete.
                setTimeout(function () {
                  $this.props.progressBars.removeRecord(id);
                }, 1000);
              },
              error: function (data) {
                var response = JSON.parse(data.responseText);
                $this.props.progressBars.addRecord(
                    {error: true, filename: file.name, id: id,
                      message: response.error});
                setTimeout(function () {
                  $this.props.progressBars.removeRecord(id);
                }, 1000);
              }
            });
          };

          $.ajax({
            url: this.props.uploadPath + "?terminal_sid=" + this.state.sid +
                 "&filename=" + files[i].name,
            success: function (file) {
              return function (data) {
                postFile(file);
              };
            }(files[i]),
            error: function (file) {
              return function (data) {
                var id = randomID();
                var response = JSON.parse(data.responseText);
                $this.props.progressBars.addRecord(
                    {error: true, filename: file.name, id: id,
                     message: response.error});
              }
            }(files[i]),
            type: "GET"
          });
        }
        $el.find(".terminal-drop-overlay").css("display", "none");
      }.bind(this));
    }.bind(this);

    var bindResizeEvent = function () {
      // Calculate terminal and app-window width/height relation.  Used for
      // resize procedure we will hide right and bottom border of teriminal.
      // and add the same size to app-window for good looking and resize
      // indicator
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

      // initial app-window size
      el.style.width = term.element.clientWidth + totalWidthOffset;
      el.style.height = term.element.clientHeight + totalHeightOffset;

      $el.resizable();
      $el.bind("resize", function () {
        // We use CONTROL_START and CONTROL_END to specify the control buffer
        // region.  Ghost can use the 2 characters to know the control string.
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

          // Fine tune app-window size to match terminal.
          // Prevent white space between app-window and terminal.
          el.style.width = term.element.clientWidth + totalWidthOffset;
          el.style.height = term.element.clientHeight + totalHeightOffset;

          // Send to ghost to set new size
          sock.send((new Uint8Array([CONTROL_START])).buffer);
          sock.send(JSON.stringify(msg));
          sock.send((new Uint8Array([CONTROL_END])).buffer);
        }
      });
    }

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
      bindResizeEvent();
      bindDisconnectEvent();

      // Only bind drag and drop event if uploadPath is provided
      if (typeof(this.props.uploadPath) != "undefined") {
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
  onCopyMouseUp: function (event) {
    this.term.CopyAll();
  },
  render: function () {
    var minimize_icon_node = "", copy_icon_node = "";
    if (this.props.enableMinimize) {
      copy_icon_node = (
          <div className="app-window-icon app-window-minimize"
           onMouseUp={this.onMinimizeMouseUp}></div>
      );
    }
    if (this.props.enableCopy) {
      copy_icon_node = (
          <div className="app-window-icon app-window-copy"
           onMouseUp={this.onCopyMouseUp}></div>
      );
    }
    return (
      <div className="app-window" ref="window"
          onMouseDown={this.onWindowMouseDown}>
        <div className="app-window-title">{this.props.title}</div>
        <div className="app-window-control">
          {copy_icon_node}
          {minimize_icon_node}
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

var UploadProgress = React.createClass({
  getInitialState: function () {
    return {records: []};
  },
  addRecord: function (record) {
    this.setState(function (state, props) {
      state.records.push(record);
    });
  },
  removeRecord: function (id) {
    this.setState(function (state, props) {
      var index = state.records.findIndex(function (el, index, array) {
        return el.id == id;
      });
      if (index !== -1) {
        state.records.splice(index, 1);
      }
    });
  },
  render: function () {
    var display = "";
    if (this.state.records.length == 0) {
      display = "upload-progress-bars-hidden";
    }
    return (
        <div className={"upload-progress-bars panel panel-warning " + display}>
          <div className="panel-heading">Upload Progress</div>
          <div className="panel-body upload-progress-panel-body">
          {
            this.state.records.map(function (record) {
              if (record.error) {
                return <ErrorBar progress={this} record={record} />;
              } else {
                return <ProgressBar record={record} />;
              }
            }.bind(this))
          }
          </div>
        </div>
    );
  }
});


var ProgressBar = React.createClass({
  render: function () {
    return (
        <div className="progress">
          <div className="progress-bar upload-progress-bar"
            id={this.props.record.id} role="progressbar"
            aria-valuenow="0" aria-valuemin="0" aria-valuemax="100">
            <span className="percent">0%</span> - {this.props.record.filename}
          </div>
        </div>
    );
  }
});


var ErrorBar = React.createClass({
  onCloseClicked: function () {
    this.props.progress.removeRecord(this.props.record.id);
  },
  render: function () {
    return (
        <div className="progress-error">
          <div className="alert alert-danger upload-alert">
            <button type="button" className="close" aria-label="Close"
              onClick={this.onCloseClicked}>
              <span aria-hidden="true">&times;</span>
            </button>
            <b>{this.props.record.filename}</b><br />
              {this.props.record.message}
          </div>
        </div>
    );
  }
});
