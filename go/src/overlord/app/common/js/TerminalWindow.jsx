// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// Terminal Window widget
//
// - TerminalWindow
//   props:
//     id: DOM id
//     path: path to websocket
//     uploadPath: path to upload the file (without terminal sid)
//     title: window title
//     onOpen: callback for connection open
//     onClose: callback for connection close
//     onError: callback for connection error
//     onMessage: callback for message (binary)
//     onMessage: callback for control message (JSON)
//     onCloseClicked: callback for close button clicked
//
// - UploadProgress
//   - ProgressBar


var TerminalWindow = React.createClass({
  randomID: function() {
    return Math.random().toString(36).replace(/[^a-z]+/g, '').substr(0, 6);
  },
  getInitialState: function() {
    return {sid: ""};
  },
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

    var term = new Terminal({
      cols: 80,
      rows: 24,
      useStyle: true,
      screenKeys: true
    });

     var bindDragAndDropEvents = function() {
      var termDom = $el.find(".terminal");
      var overlay = $el.find(".terminal-drop-overlay");

      termDom.on("dragenter", function (e) {
        e.preventDefault();
        e.stopPropagation();
        $el.find(".terminal-drop-overlay").css("display", "block");
      }.bind(this));

      overlay.on("dragenter", function (e) {
        e.preventDefault();
        e.stopPropagation();
      });

      overlay.on("dragover", function (e) {
        e.preventDefault();
        e.stopPropagation();
      });

      overlay.on("dragleave", function (e) {
        e.preventDefault();
        e.stopPropagation();
        $el.find(".terminal-drop-overlay").css("display", "none");
      });

      overlay.on("drop", function (e) {
        e.preventDefault();
        e.stopPropagation();

        var files = e.originalEvent.dataTransfer.files;
        for (var i = 0; i < files.length; i++) {
          var id = this.randomID();
          this.props.progressBars.addRecord({filename: files[i].name, id: id});
          var formData = new FormData();
          formData.append("file", files[i]);

          $.ajax({
            xhr: function(file, id) {
              return function() {
                var xhr = new window.XMLHttpRequest();
                xhr.upload.addEventListener("progress", function(e) {
                  if (e.lengthComputable) {
                    var percentComplete = Math.round(e.loaded * 100 / e.total);
                    $('#' + id).css('width', percentComplete + '%');
                    $('#' + id + ' > .percent').text(percentComplete + '%');
                  }
                }, false);
                return xhr;
              }
            }(files[i], id),
            url: this.props.uploadPath + "?sid=" + this.state.sid,
            data: formData,
            cache: false,
            contentType: false,
            processData: false,
            type: 'POST',
            success: function (id) {
              return function (data) {
                $('#' + id).css('width', '100%');
                // Display the progressbar for 1 more seconds after complete.
                setTimeout(function () {
                  this.props.progressBars.removeRecord(id);
                }.bind(this), 1000);
              }.bind(this);
            }.bind(this)(id)
          });
        }
        $el.find(".terminal-drop-overlay").css("display", "none");
      }.bind(this));
    }.bind(this);

    var bindResizeEvent = function() {
      // Calculate terminal and terminal-window width/height relation.  Used for
      // resize procedure we will hide right and bottom border of teriminal.
      // and add the same size to terminal-window for good looking and resize
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

      // initial terminal-window size
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
    }

    sock.onopen = function (e) {
      term.open(el);
      term.on("title", function (title) {
        $el.find(".terminal-title").text(title);
      });

      term.on("data", function (data) {
        sock.send(data);
      });

      sock.onmessage = function (msg) {
        if (msg.data instanceof Blob) {
          var callback = this.props.onMessage;
          ReadBlobAsText(msg.data, function(text) {
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

      sock.onclose = function (e) {
        var callback = this.props.onClose;
        if (typeof(callback) != "undefined") {
          (callback.bind(this))(e);
        }
      }.bind(this);

      // Bind events
      bindResizeEvent();

      // Only bind drag and drop event if uploadPath is provided
      if (typeof(this.props.uploadPath) != "undefined") {
        bindDragAndDropEvents();
      }

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
        <div className="terminal-drop-overlay">
          Drop files here to upload
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
    this.state.records.push(record);
    this.forceUpdate();
  },
  removeRecord: function (id) {
    var records = this.state.records;
    for (var i = 0; i < records.length; ++i) {
      if (records[i].id == id) {
        records.splice(i, 1);
        break;
      }
    }
    this.forceUpdate();
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
              return (
                  <ProgressBar record={record}>
                  </ProgressBar>
              );
            })
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
