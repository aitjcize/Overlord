// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// View for Fixture dashboard App
//
// Requires:
//   NavBar.jsx :: NavBar
//   TerminalWindow.jsx :: TerminalWindow
//
// - App
//  - NavBar
//  - Fixture
//    - Lights
//    - Terminals
//    - Controls
//    - MainLog
//    - AuxLogs
//      - AuxLog


LOG_BUF_SIZE = 8192

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
    if (data.properties.context != "fixture") {
      return;
    }
    var ip = data.properties.ip;
    if (typeof(this.state.fixture_groups[ip]) == "undefined") {
      this.state.fixture_groups[ip] = [];
    }
    this.clientIPMap[data.mid] = ip;
    this.state.fixture_groups[ip].push(data);
    this.forceUpdate();
  },
  removeClient: function (data) {
    var ip = this.clientIPMap[data.mid];
    var group = this.state.fixture_groups[ip];

    if (typeof(group) == "undefined") {
      return;
    }

    for (var i = 0; i < group.length; i++) {
      if (group[i].mid == data.mid) {
        group.splice(i, 1);

        if (group.length == 0) {
          delete this.state.fixture_groups[ip];
        }
        this.forceUpdate();
        return;
      }
    }
    return;
  },
  addTerminal: function (mid) {
    if (typeof(this.state.terminals[mid]) == "undefined") {
      this.state.terminals[mid] = true;
    }
    this.forceUpdate();
  },
  removeTerminal: function (mid) {
    if (typeof(this.state.terminals[mid]) != "undefined") {
      delete this.state.terminals[mid];
    }
    this.forceUpdate();
  },
  getInitialState: function () {
    this.clientIPMap = {};
    return {fixture_groups: {}, terminals: {}};
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
  },
  render: function () {
    onClose = function (e) {
      this.props.app.removeTerminal(this.props.mid);
    }
    return (
      <div id="main">
        <NavBar name="Fixture Dashboard" url="/api/apps/list" />
        <div className="terminals">
          {
            Object.keys(this.state.terminals).map(function (mid) {
              return (
                <TerminalWindow key={mid} mid={mid} id={"terminal-" + mid}
                 title={mid} path={"/api/agent/pty/" + mid}
                 onClose={onClose} app={this} />
              );
            }.bind(this))
          }
        </div>
        <div className="app-box panel panel-info">
          <div className="panel-heading">Clients</div>
          <div className="panel-body">
            {
              Object.keys(this.state.fixture_groups).map(function (key) {
                var group = this.state.fixture_groups[key];
                return (
                  <Fixture key={key} members={group} app={this}/>
                );
              }.bind(this))
            }
          </div>
        </div>
      </div>
    );
  }
});

var Fixture = React.createClass({
  render: function () {
    var ip = this.props.members[0].properties.ip;
    return (
      <div className="fixture-block panel panel-success">
        <div className="panel-heading">{ip}</div>
        <div className="panel-body">
          <Lights ref="lights" members={this.props.members} />
          <Terminals members={this.props.members} app={this.props.app}/>
          <Controls members={this.props.members} fixture={this} />
          <MainLog ref="mainlog" fixture={this} ip={ip} />
          <AuxLogs members={this.props.members} fixture={this} />
        </div>
      </div>
    );
  }
});

var Lights = React.createClass({
  updateLightStatus: function (id, status_class) {
    $(this.refs[id].getDOMNode()).attr("class", "label status-light " + status_class);
  },
  scanForLightMsg: function (msg) {
    var patt = /LIGHT\[(.*)\]\s*=\s*'(\S*)'/g;
    var found;
    while (found = patt.exec(msg)) {
      this.updateLightStatus(found[1], found[2]);
    }
  },
  render: function () {
    var ip = this.props.members[0].properties.ip;
    var members = this.props.members;
    var status_lights = [];
    for (var i = 0; i < members.length; i++) {
      if (typeof(members[i].properties.ui) != "undefined" &&
          typeof(members[i].properties.ui.lights) != "undefined") {
        status_lights = status_lights.concat(members[i].properties.ui.lights);
      }
    }
    return (
      <div className="status-block well well-sm">
      {
        status_lights.map(function (light) {
          return (
            <span key={light[0]} className={"label status-light " + light[2]}
              ref={light[0]}>
              {light[1]}
            </span>
          );
        })
      }
      </div>
    );
  }
});

var Terminals = React.createClass({
  onTerminalClick: function (e) {
    var target = $(e.target);
    this.props.app.addTerminal(target.data("mid"));
  },
  render: function () {
    var members = this.props.members;
    var terminals = [];
    for (var i = 0; i < members.length; i++) {
      if (typeof(members[i].properties.terminal) != "undefined") {
        terminals.push([members[i].mid, members[i].properties.terminal]);
      }
    }
    return (
      <div className="status-block well well-sm">
      {
        terminals.map(function (pair) {
          var mid = pair[0];
          var name = pair[1];
          return (
            <button key={mid} className="btn btn-xs btn-info" data-mid={mid}
                onClick={this.onTerminalClick}>
            {name}
            </button>
          );
        }.bind(this))
      }
      </div>
    );
  }
});

var Controls = React.createClass({
  executeRemoteCmd: function (mid, cmd) {
    var url = "ws" + ((window.location.protocol == "https:")? "s": "" ) +
              "://" + window.location.host + "/api/agent/shell/" + mid +
              "?command=" + encodeURIComponent(cmd);
    var sock = new WebSocket(url);

    sock.onopen = function () {
      sock.onmessage = function (msg) {
        if (msg.data instanceof Blob) {
          ReadBlobAsText(msg.data, function(text) {
            this.props.fixture.refs.mainlog.appendLog(text);
          }.bind(this));
        }
      }.bind(this)
    }.bind(this)
  },
  onCommandClicked: function (e) {
    var target = $(e.target);
    this.executeRemoteCmd(target.data('mid'), target.data('cmd'));
  },
  render: function () {
    var members = this.props.members;
    var member_controls = [];
    for (var i = 0; i < members.length; i++) {
      if (typeof(members[i].properties.ui) != "undefined" &&
          typeof(members[i].properties.ui.controls) != "undefined") {
        member_controls.push([members[i].mid, members[i].properties.ui.controls]);
      }
    }
    return (
      <div className="controls-block well well-sm">
      {
        member_controls.map(function (el) {
          var mid = el[0];
          var controls = el[1];
          return controls.map(function (control) {
            if (typeof(control[1]) != "string") { // submenu
              return (
                <div className="well well-sm well-inner" key={control[0]}>
                {control[0]}<br />
                {
                  control[1].map(function (ctrl) {
                    return (
                      <button key={ctrl[0]}
                          className="command-btn btn btn-xs btn-warning"
                          data-mid={mid} data-cmd={ctrl[1]}
                          onClick={this.onCommandClicked}>
                        {ctrl[0]}
                      </button>
                    );
                  }.bind(this))
                }
                </div>
              );
            }
            return (
              <div key={control[0]}
                  className="command-btn btn btn-xs btn-primary"
                  data-mid={mid} data-cmd={control[1]}
                  onClick={this.onCommandClicked}>
                {control[0]}
              </div>
            );
          }.bind(this))
        }.bind(this))
      }
      </div>
    );
  }
});

var MainLog = React.createClass({
  appendLog: function (text) {
    var odiv = this.odiv;
    this.props.fixture.refs.lights.scanForLightMsg(text);
    odiv.innerText += text;
    if (odiv.innerText.length > LOG_BUF_SIZE) {
      odiv.innerText = odiv.innerText.substr(odiv.innerText.length -
                                             LOG_BUF_SIZE, LOG_BUF_SIZE);
    }
    odiv.scrollTop = odiv.scrollHeight;
  },
  componentDidMount: function () {
    this.odiv = this.refs["log-" + this.props.ip].getDOMNode();
  },
  render: function () {
    return (
      <div className="log log-main well well-sm" ref={"log-" + this.props.ip}>
      </div>
    );
  }
});

var AuxLogs = React.createClass({
  render: function () {
    var ip = this.props.members[0].properties.ip;
    var members = this.props.members;
    var log_pairs = [];
    for (var i = 0; i < members.length; i++) {
      if (typeof(members[i].properties.ui) != "undefined" &&
          typeof(members[i].properties.ui.log) != "undefined") {
        log_pairs.push([members[i].mid, members[i].properties.ui.log]);
      }
    }
    return (
      <div className="log-block">
        {
          log_pairs.map(function (pair) {
            var mid = pair[0];
            var logs = pair[1];
            return logs.map(function (filename) {
              return (
                <AuxLog mid={mid} filename={filename} fixture={this.props.fixture}/>
              )
            }.bind(this))
          }.bind(this))
        }
      </div>
    );
  }
});

var AuxLog = React.createClass({
  componentDidMount: function () {
    var url = "ws" + ((window.location.protocol == "https:")? "s": "" ) +
              "://" + window.location.host + "/api/agent/shell/" +
              this.props.mid + "?command=" +
              encodeURIComponent("tail -f " + this.props.filename);
    var sock = new WebSocket(url);

    sock.onopen = function () {
      var odiv = this.refs["log-" + this.props.mid].getDOMNode();
      sock.onmessage = function (msg) {
        if (msg.data instanceof Blob) {
          ReadBlobAsText(msg.data, function (text) {
            this.props.fixture.refs.lights.scanForLightMsg(text);
            odiv.innerText += text;
            if (odiv.innerText.length > LOG_BUF_SIZE) {
              odiv.innerText = odiv.innerText.substr(odiv.innerText.length -
                                                     LOG_BUF_SIZE, LOG_BUF_SIZE);
            }
            odiv.scrollTop = odiv.scrollHeight;
          }.bind(this));
        }
      }.bind(this)
    }.bind(this)
  },
  render: function () {
    return (
      <div className="log log-aux well well-sm" ref={"log-" + this.props.mid}>
      </div>
    );
  }
});

React.render(
  <App url="/api/agents/list" pollInterval={60000} />,
  document.body
);
