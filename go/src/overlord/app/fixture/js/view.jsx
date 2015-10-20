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
DEFAULT_LIGHT_POLL_INTERVAL = 3000

var LIGHT_CSS_MAP = {
  'light-toggle-off': 'label-danger',
  'light-toggle-on': 'label-success'
};

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
    var onClose = function (e) {
      this.props.app.removeTerminal(this.props.id);
      this.props.app.socket.emit("unsubscribe", this.terminal_sid);
    };
    return (
      <div id="main">
        <NavBar name="Fixture Dashboard" url="/api/apps/list" />
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
                 onControl={onControl} onClose={onClose} />
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
                  <Fixture key={data.mid} client={data} app={this}/>
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
  executeRemoteCmd: function (mid, cmd) {
    var url = "ws" + ((window.location.protocol == "https:")? "s": "" ) +
              "://" + window.location.host + "/api/agent/shell/" + mid +
              "?command=" + encodeURIComponent(cmd);
    var sock = new WebSocket(url);

    sock.onopen = function () {
      sock.onmessage = function (msg) {
        if (msg.data instanceof Blob) {
          ReadBlobAsText(msg.data, function(text) {
            this.refs.mainlog.appendLog(text);
          }.bind(this));
        }
      }.bind(this)
    }.bind(this)
  },
  render: function () {
    var client = this.props.client;
    return (
      <div className="fixture-block panel panel-success">
        <div className="panel-heading text-center">{abbr(client.mid, 60)}</div>
        <div className="panel-body">
          <Lights ref="lights" client={this.props.client} fixture={this} />
          <Terminals client={client} app={this.props.app} />
          <Controls ref="controls" client={client} fixture={this} />
          <MainLog ref="mainlog" fixture={this} id={client.mid} />
          <AuxLogs client={client} fixture={this} />
        </div>
      </div>
    );
  }
});

var Lights = React.createClass({
  updateLightStatus: function (id, status_class) {
    var node = $(this.refs[id].getDOMNode());
    node.removeClass(this.refs[id].props.prevLight);
    node.addClass(status_class);
    this.refs[id].props.prevLight = status_class;
  },
  scanForLightMsg: function (msg) {
    var patt = /LIGHT\[(.*)\]\s*=\s*'(\S*)'/g;
    var found;
    while (found = patt.exec(msg)) {
      this.updateLightStatus(found[1], LIGHT_CSS_MAP[found[2]]);
    }
  },
  componentDidMount: function() {
    var client = this.props.client;
    var update_command;

    if (typeof(client.properties.ui) != "undefined") {
      update_command = client.properties.ui.lights.update_command;
    }
    setTimeout(function() {
      this.props.fixture.executeRemoteCmd(client.mid, update_command);
    }.bind(this), 5000);
  },
  render: function () {
    var client = this.props.client;
    var lights = [];

    if (typeof(client.properties.ui) != "undefined") {
      lights = client.properties.ui.lights.items || [];
    }
    return (
      <div className="status-block well well-sm">
      {
        lights.map(function (light) {
          var extra_css = "";
          var extra = {};
          if (typeof(light.command) != "undefined") {
            extra_css = "status-light-clickable";
            extra.onClick = function() {
              this.props.fixture.executeRemoteCmd(client.mid, light.command);
            }.bind(this);
          }
          var light_css = LIGHT_CSS_MAP[light.light];
          return (
            <span key={light.id} className={"label " + extra_css + " " +
              light_css} prevLight={light_css} ref={light.id} {...extra}>
              {light.label}
            </span>
          );
        }.bind(this))
      }
      </div>
    );
  }
});

var Terminals = React.createClass({
  getCmdOutput: function (mid, cmd) {
    var url = "ws" + ((window.location.protocol == "https:")? "s": "" ) +
              "://" + window.location.host + "/api/agent/shell/" + mid +
              "?command=" + cmd;
    var sock = new WebSocket(url);
    var deferred = $.Deferred();

    sock.onopen = function (e) {
      var blobs = [];
      sock.onmessage = function (msg) {
        if (msg.data instanceof Blob) {
          blobs.push(msg.data);
        }
      }
      sock.onclose = function (e) {
        var value = "";
        if (blobs.length == 0) {
          deferred.resolve("");
        }
        for (var i = 0; i < blobs.length; i++) {
          ReadBlobAsText(blobs[i], function(current) {
            return function(text) {
              value += text;
              if (current == blobs.length - 1) {
                deferred.resolve(value);
              }
            }
          }(i));
        }
      }
    }
    return deferred.promise();
  },
  onTerminalClick: function (e) {
    var target = $(e.target);
    var mid = target.data("mid");
    var term = target.data("term");
    var id = mid + "::" + term.name;

    // Add mid reference to term object
    term.mid = mid;

    if (typeof(term.path_cmd) != "undefined" &&
        term.path_cmd.match(/^\s+$/) == null) {
      this.getCmdOutput(mid, term.path_cmd).done(function(path) {
        if (path.replace(/^\s+|\s+$/g, "") == "") {
          alert("This TTY device does not exist!");
        } else {
          term.path = path;
          this.props.app.addTerminal(id, term);
        }
      }.bind(this));
      return;
    }

    this.props.app.addTerminal(id, term);
  },
  render: function () {
    var client = this.props.client;
    var terminals = [];

    if (typeof(client.properties.ui) != "undefined") {
      terminals = client.properties.ui.terminals || [];
    }
    return (
      <div className="status-block well well-sm">
      {
        terminals.map(function (term) {
          return (
            <button className="btn btn-xs btn-info" data-mid={client.mid}
                data-term={JSON.stringify(term)} onClick={this.onTerminalClick}>
            {term.name}
            </button>
          );
        }.bind(this))
      }
      </div>
    );
  }
});

var Controls = React.createClass({
  onCommandClicked: function (e) {
    var target = $(e.target);
    var ctrl = target.data("ctrl");
    if (ctrl.type == "toggle") {
      if (target.hasClass("active")) {
        this.props.fixture.executeRemoteCmd(target.data("mid"), ctrl.off_command);
        target.removeClass("active");
      } else {
        this.props.fixture.executeRemoteCmd(target.data("mid"), ctrl.on_command);
        target.addClass("active");
      }
    } else {
      this.props.fixture.executeRemoteCmd(target.data("mid"), ctrl.command);
    }
  },
  render: function () {
    var client = this.props.client;
    var controls = [];
    var mid = client.mid;

    if (typeof(client.properties.ui) != "undefined") {
      controls = client.properties.ui.controls || [];
    }
    return (
      <div className="controls-block well well-sm">
      {
        controls.map(function (control) {
          if (typeof(control.group) != "undefined") { // sub-group
            return (
              <div className="well well-sm well-inner" key={control.name}>
              {control.name}<br />
              {
                control.group.map(function (ctrl) {
                  return (
                    <button key={ctrl.name}
                        className="command-btn btn btn-xs btn-warning"
                        data-mid={mid} data-ctrl={JSON.stringify(ctrl)}
                        onClick={this.onCommandClicked}>
                      {ctrl.name}
                    </button>
                  );
                }.bind(this))
              }
              </div>
            );
          }
          return (
            <div key={control.name}
                className="command-btn btn btn-xs btn-primary"
                data-mid={mid} data-ctrl={JSON.stringify(control)}
                onClick={this.onCommandClicked}>
              {control.name}
            </div>
          );
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
    this.odiv = this.refs["log-" + this.props.id].getDOMNode();
  },
  render: function () {
    return (
      <div className="log log-main well well-sm" ref={"log-" + this.props.id}>
      </div>
    );
  }
});

var AuxLogs = React.createClass({
  render: function () {
    var client = this.props.client;
    var logs = [];

    if (typeof(client.properties.ui) != "undefined") {
      logs = client.properties.ui.logs || [];
    }
    return (
      <div className="log-block">
        {
          logs.map(function (filename) {
            return (
              <AuxLog mid={client.mid} filename={filename}
               fixture={this.props.fixture}/>
            )
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
    this.sock = sock;
  },
  componentWillUnmount: function() {
    this.sock.close();
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
