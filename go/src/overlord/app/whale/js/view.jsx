// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// View for Fixture dashboard App
//
// Requires:
//   NavBar.jsx :: NavBar
//   FixtureWindow.jsx :: FixtureWindow
//   TerminalWindow.jsx :: TerminalWindow
//
// - App
//  - NavBar
//  - FixtureWindow

var App = React.createClass({
  loadClientsFromServer: function () {
    $.ajax({
      url: this.props.url,
      dataType: "json",
      success: function (data) {
        for (var i = 0; i < data.length; i++) {
          this.addClient(data[i]);
        }
      }.bind(this),
      error: function (xhr, status, err) {
        console.error(this.props.url, status, err.toString());
      }.bind(this)
    });
  },
  fetchProperties: function (mid) {
    var result = undefined;
    var url = '/api/agent/properties/' + mid;
    $.ajax({
      url: url,
      async: false,
      dataType: "json",
      success: function (data) {
        result = data;
      }.bind(this),
      error: function (xhr, status, err) {
        console.error(url, status, err.toString());
      }.bind(this)
    });
    return result;
  },
  addClient: function (data) {
    // Data should have the format {mid: "mid", sid: "sid"}
    data.properties = this.fetchProperties(data.mid);
    if (typeof(data.properties) == "undefined" ||
        typeof(data.properties.context) == "undefined" ||
        data.properties.context.indexOf("whale") === -1) {
      return;
    }
    this.state.fixtures.push(data);
    this.forceUpdate();
  },
  removeClient: function (data) {
    var fixtures = this.state.fixtures;
    for (var i = 0; i < fixtures.length; i++) {
      if (fixtures[i].mid == data.mid) {
        fixtures.splice(i, 1);
        this.forceUpdate();
        return;
      }
    }
    return;
  },
  addTerminal: function (id, term) {
    this.state.terminals[id] = term;
    this.forceUpdate();
  },
  removeTerminal: function (id) {
    if (typeof(this.state.terminals[id]) != "undefined") {
      delete this.state.terminals[id];
    }
    this.forceUpdate();
  },
  getInitialState: function () {
    return {fixtures: [], terminals: {}};
  },
  componentDidMount: function () {
    this.loadClientsFromServer();

    var socket = io(window.location.protocol + "//" + window.location.host,
                    {path: "/api/socket.io/"});
    socket.on("agent joined", function (msg) {
      var obj = JSON.parse(msg);
      this.addClient(obj);
    }.bind(this));

    socket.on("agent left", function (msg) {
      var obj = JSON.parse(msg);
      this.removeClient(obj);
    }.bind(this));

    // Initiate a file download
    socket.on("file download", function (sid) {
      var url = window.location.protocol + "//" + window.location.host +
                "/api/file/download/" + sid;
      $("<iframe id='" + sid + "' src='" + url + "' style='display:none'>" +
        "</iframe>").appendTo('body');
    });
    this.socket = socket;
  },
  render: function () {
    var onControl = function (control) {
      if (control.type == "sid") {
        this.terminal_sid = control.data;
        this.props.app.socket.emit("subscribe", control.data);
      }
    };
    var onCloseClicked = function (e) {
      this.props.app.removeTerminal(this.props.id);
      this.props.app.socket.emit("unsubscribe", this.terminal_sid);
    };
    return (
      <div id="main">
        <NavBar name="Whale Fixture Dashboard" url="/api/apps/list" />
        <div className="terminals">
          {
            Object.keys(this.state.terminals).map(function (id) {
              var term = this.state.terminals[id];
              var extra = "";
              if (typeof(term.path) != "undefined") {
                extra = "?tty_device=" + term.path;
              }
              return (
                <TerminalWindow key={id} mid={term.mid} id={id} title={id}
                 path={"/api/agent/tty/" + term.mid + extra}
                 uploadPath={"/api/agent/upload/" + term.mid}
                 app={this} progressBars={this.refs.uploadProgress}
                 onControl={onControl} onCloseClicked={onCloseClicked} />
              );
            }.bind(this))
          }
        </div>
        <div className="upload-progress">
          <UploadProgress ref="uploadProgress" />
        </div>
        <div className="app-box panel panel-info">
          <div className="panel-heading">Clients</div>
          <div className="panel-body">
            {
              this.state.fixtures.map(function (data) {
                return (
                  <FixtureWindow key={data.mid} client={data} app={this}/>
                );
              }.bind(this))
            }
          </div>
        </div>
      </div>
    );
  }
});

React.render(
  <App url="/api/agents/list" pollInterval={60000} />,
  document.body
);
