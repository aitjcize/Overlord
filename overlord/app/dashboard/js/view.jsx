// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// Requires:
//   NavBar.jsx :: NavBar
//   UploadProgressWidget.jsx :: UploadProgressWidget
//   FixtureWidget.jsx :: FixtureWidget
//   TerminalWindow.jsx :: TerminalWindow
//   CameraWindow.jsx :: CameraWindow
//
// View for Dashboard App:
// - App
//  - NavBar
//  - SideBar
//   - ClientBox
//    - FilterInput
//    - ClientList
//     - [ClientInfo ...]
//    - RecentList
//     - ClientInfo
//  - Windows
//   - [TerminalWindow ...]
//   - [CameraWindow ...]
//   - UploadProgressWidget
//  - FixtureGroup
//   - [FixtureWidget ...]

var App = React.createClass({
  mixins: [BaseApp],
  addTerminal: function (id, term) {
    this.setState(function (state, props) {
      state.terminals[id] = term;
    });
  },
  addFixture: function (client) {
    if (this.isClientInList(this.state.fixtures, client)) {
      return;
    }

    // compute how many fixtures we can put in the screen
    var screen = {
        width: window.innerWidth,
    };

    var sidebar = ReactDOM.findDOMNode(this.refs.sidebar).getBoundingClientRect();

    screen.width -= sidebar.right;

    var nFixturePerRow = Math.floor(
       screen.width / (FIXTURE_WINDOW_WIDTH + FIXTURE_WINDOW_MARGIN * 2));
    nFixturePerRow = Math.max(1, nFixturePerRow);
    var nTotalFixture = Math.min(2 * nFixturePerRow, 8);

    // only keep recently opened @nTotalFixture fixtures.
    this.setState(function (state, props) {
      state.fixtures.push(client);
      return {fixtures: state.fixtures.slice(-nTotalFixture)};
    });
  },
  addCamera: function (id, cam) {
    this.setState(function (state, props) {
      state.cameras[id] = cam;
    });
  },
  toggleFixtureState: function (client) {
    if (this.isClientInList(this.state.fixtures, client)) {
      this.removeFixture(client.mid);
    } else {
      this.addFixture(client);
    }
  },
  removeTerminal: function (id) {
    this.setState(function (state, props) {
      if (typeof(state.terminals[id]) != "undefined") {
        delete state.terminals[id];
      }
    });
  },
  removeFixture: function (id) {
    this.setState(function (state, props) {
      this.removeClientFromList(state.fixtures, {mid: id});
    });
  },
  removeCamera: function (id) {
    this.setState(function (state, props) {
      if (typeof(state.cameras[id]) != "undefined") {
        delete state.cameras[id];
      }
    });
  },
  getInitialState: function () {
    return {cameras: [], fixtures: [], recentclients: [], terminals: {}};
  },
  componentDidMount: function () {
    var socket = io(window.location.protocol + "//" + window.location.host,
                    {path: "/api/socket.io/"});
    this.socket = socket;

    socket.on("agent joined", function (msg) {
      var client = JSON.parse(msg);
      this.addClient(client);

      this.state.recentclients.splice(0, 0, client);
      this.state.recentclients = this.state.recentclients.slice(0, 5);
    }.bind(this));

    socket.on("agent left", function (msg) {
      var client = JSON.parse(msg);

      this.removeClientFromList(this.state.clients, client);
      this.removeClientFromList(this.state.recentclients, client);
      this.removeFixture(client.mid);
    }.bind(this));

    // Initiate a file download
    socket.on("file download", function (sid) {
      var url = window.location.protocol + "//" + window.location.host +
                "/api/file/download/" + sid;
      $("<iframe src='" + url + "' style='display:none'>" +
        "</iframe>").appendTo('body');
    });
  },
  render: function () {
    return (
      <div id="main">
        <NavBar name="Dashboard" url="/api/apps/list" ref="navbar" />
        <div id="container">
          <SideBar clients={this.getFilteredClientList()} ref="sidebar"
              recentclients={this.state.recentclients} app={this} />
          <FixtureGroup data={this.state.fixtures} app={this}
           uploadProgress={this.refs.uploadProgress} />
        </div>
        <div className="windows">
          <Windows app={this} terminals={this.state.terminals}
           uploadProgress={this.refs.uploadProgress}
           cameras={this.state.cameras} />
        </div>
        <div className="upload-progress">
          <UploadProgressWidget ref="uploadProgress" />
        </div>
      </div>
    );
  }
});

var SideBar = React.createClass({
  render: function () {
    return (
      <div className="sidebar">
        <ClientBox data={this.props.clients} app={this.props.app} />
        <RecentList data={this.props.recentclients} app={this.props.app} />
      </div>
    );
  }
});

var ClientBox = React.createClass({
  render: function () {
    return (
      <div className="client-box panel panel-success">
        <div className="panel-heading">Clients</div>
        <div className="panel-body">
          <FilterInput app={this.props.app} />
          <ClientList data={this.props.data} app={this.props.app} />
        </div>
      </div>
    );
  }
})

var FilterInput = React.createClass({
  onKeyUp: function (event) {
    this.props.app.setDisplayFilterPattern(this.refs.filter.value);
  },
  render: function () {
    return (
      <div>
        <input type="text" className="filter-input form-control" ref="filter"
            placeholder="keyword" onKeyUp={this.onKeyUp}></input>
      </div>
    )
  }
});

var ClientList = React.createClass({
  render: function () {
    return (
      <div className="list-box client-list">
        {
          this.props.data.map(function (item) {
            return (
              <ClientInfo key={item.mid} data={item} app={this.props.app}>
                {displayClient(item)}
              </ClientInfo>
              );
          }.bind(this))
        }
      </div>
    );
  }
});

var RecentList = React.createClass({
  render: function () {
    return (
      <div className="recent-box panel panel-info">
        <div className="panel-heading">Recent Connected Clients</div>
        <div className="panel-body">
          <div className="list-box recent-list">
            {
              this.props.data.map(function (item) {
                return (
                  <ClientInfo key={item.mid} data={item} app={this.props.app}>
                    {displayClient(item)}
                  </ClientInfo>
                  );
              }.bind(this))
            }
          </div>
        </div>
      </div>
    )
  }
});

var ClientInfo = React.createClass({
  openTerminal: function (event) {
    this.props.app.addTerminal(randomID(), this.props.data);
  },
  openCamera: function (event) {
    this.props.app.addCamera(this.props.data.mid, this.props.data);
  },
  onUIButtonClick: function (event) {
    this.props.app.toggleFixtureState(this.props.data);
  },
  componentDidMount: function (event) {
    // Since the button covers the machine ID text, abbrieviate to match the
    // current visible width.
    var chPerLine = 50;
    var pxPerCh = this.refs.mid.clientWidth / chPerLine;
    this.refs.mid.innerText =
      abbr(this.refs.mid.innerText,
           chPerLine - (this.refs["info-buttons"].clientWidth)/ pxPerCh);
  },
  render: function () {
    var display = "block";
    var ui_span = null;
    var cam_span = null;

    if (typeof(this.props.data.properties) != "undefined" &&
        typeof(this.props.data.properties.context) != "undefined" &&
        this.props.data.properties.context.indexOf("ui") !== -1) {
      var ui_state = this.props.app.isClientInList(
          this.props.app.state.fixtures, this.props.data);
      var ui_light_css = LIGHT_CSS_MAP[ui_state ? "light-toggle-on"
                                                : "light-toggle-off"];
      ui_span = (
        <div className={"label " + ui_light_css + " client-info-button"}
            data-mid={this.props.data.key} onClick={this.onUIButtonClick}>
          UI
        </div>
      );
    }
    if (typeof(this.props.data.properties) != "undefined" &&
        typeof(this.props.data.properties.context) != "undefined" &&
        this.props.data.properties.context.indexOf("cam") !== -1) {
      cam_span = (
        <div className="label label-success client-info-button"
            data-mid={this.props.data.key} onClick={this.openCamera}>
          CAM
        </div>
      );
    }
    return (
      <div className="client-info">
        <div className="client-info-mid" ref="mid">
          {this.props.children}
        </div>
        <div className="client-info-buttons" ref="info-buttons">
          {cam_span}
          {ui_span}
          <div className="label label-warning client-info-button"
              data-mid={this.props.data.key} onClick={this.openTerminal}>
            Terminal
          </div>
        </div>
      </div>
    );
  }
});

var Windows = React.createClass({
  render: function () {
    var onTerminalControl = function (control) {
      if (control.type == "sid") {
        this.terminal_sid = control.data;
        this.props.app.socket.emit("subscribe", control.data);
      }
    };
    var onTerminalCloseClicked = function (event) {
      this.props.app.removeTerminal(this.props.id);
      this.props.app.socket.emit("unsubscribe", this.terminal_sid);
    };
    var onCameraCloseClicked = function (event) {
      this.props.app.removeCamera(this.props.id);
    }
    // We need to make TerminalWindow and CameraWindow have the same parent
    // div so z-index stacking works.
    return (
      <div>
        <div className="windows">
          {
            Object.keys(this.props.terminals).map(function (id) {
              var term = this.props.terminals[id];
              var extra = "";
              if (typeof(term.path) != "undefined") {
                extra = "?tty_device=" + term.path;
              }
              return (
                <TerminalWindow key={id} mid={term.mid} id={id} title={term.mid}
                 path={"/api/agent/tty/" + term.mid + extra}
                 uploadRequestPath={"/api/agent/upload/" + term.mid}
                 enableMaximize={true}
                 app={this.props.app} progressBars={this.props.uploadProgress}
                 onControl={onTerminalControl}
                 onCloseClicked={onTerminalCloseClicked} />
              );
            }.bind(this))
          }
          {
            Object.keys(this.props.cameras).map(function (id) {
              var cam = this.props.cameras[id];
              var cam_prop = cam.properties.camera;
              if (typeof(cam_prop) != "undefined") {
                var command = cam_prop.command;
                var width = cam_prop.width || 640;
                var height = cam_prop.height || 640;
                return (
                    <CameraWindow key={id} mid={cam.mid} id={id} title={cam.mid}
                     path={"/api/agent/shell/" + cam.mid + "?command=" +
                           encodeURIComponent(command)}
                     width={width} height={height} app={this.props.app}
                     onCloseClicked={onCameraCloseClicked} />
                );
              }
            }.bind(this))
          }
        </div>
      </div>
    );
  }
});

var FixtureGroup = React.createClass({
  render: function () {
    return (
      <div className="fixture-group">
        {
          this.props.data.map(function (item) {
            return (
              <FixtureWidget key={item.mid} client={item}
               progressBars={this.props.uploadProgress}
               app={this.props.app} />
            );
          }.bind(this))
        }
      </div>
    );
  }
});

ReactDOM.render(
  <App url="/api/agents/list" />,
  document.getElementById("body")
);
