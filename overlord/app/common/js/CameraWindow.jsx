// Copyright 2016 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// External dependencies:
// - jsmpeg.js: https://github.com/phoboslab/jsmpeg
//
// View for CameraWindow:
// - CameraWindow

var CameraWindow = React.createClass({
  mixins: [BaseWindow],
  onCloseMouseUp2: function (event) {
    this.onCloseMouseUp();
    this.sock.close();
  },
  componentDidMount: function () {
    var url = "ws" + ((window.location.protocol == "https:")? "s": "" ) +
              "://" + window.location.host + this.props.path;
    var sock = new WebSocket(url);
    var canvas = this.refs["cam-" + this.props.mid];
    var player = new jsmpeg(sock, {canvas: canvas});

    this.sock = sock;
    this.makeDraggable(".terminal");
    this.bringToFront();
  },
  render: function () {
    return (
      <div className="app-window" id={this.props.id} ref="window"
       onMouseDown={this.onWindowMouseDown}>
        <div className="app-window-title">{this.props.title}</div>
        <div className="app-window-control">
          <div className="app-window-icon app-window-close"
           onMouseUp={this.onCloseMouseUp2}></div>
        </div>
        <div className="camera-view">
          <canvas className="camera-canvas" ref={"cam-" + this.props.mid}
           width={this.props.width} height={this.props.height}></canvas>
        </div>
      </div>
    );
  }
});
