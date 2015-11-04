// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// View for Dashboard App
//
// Requires:
//   NavBar.jsx :: NavBar
//   FixtureWindow.jsx :: FixtureWindow
//   TerminalWindow.jsx :: TerminalWindow, UploadProgress
//
// - App
//  - NavBar
//  - SideBar
//    - ClientBox
//      - FilterInput
//      - ClientList
//        - ClientInfo
//    - RecentList
//      - ClientInfo
//  - FixtureGroup
//    - FixtureWindow
//  - TerminalGroup
//    - TerminalWindow
//    - UploadProgress

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
  getInitialState: function () {
    return {fixtures: [], recentclients: [], terminals: {}};
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
      $("<iframe id='" + sid + "' src='" + url + "' style='display:none'>" +
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
          <FixtureGroup data={this.state.fixtures} app={this} />
        </div>
        <TerminalGroup data={this.state.terminals} app={this} />
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
  onKeyUp: function (e) {
    this.props.app.setMidFilterPattern(this.refs.filter.value);
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
                {abbr(item.mid, 36)}
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
                    {abbr(item.mid, 36)}
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
  openTerminal: function (e) {
    this.props.app.addTerminal(this.props.data.mid, this.props.data);
  },
  onUIButtonClick: function (e) {
    this.props.app.toggleFixtureState(this.props.data);
  },
  render: function () {
    var display = "block";
    var ui_span = null;
    if (typeof(this.props.data.properties) != "undefined" &&
        typeof(this.props.data.properties.context) != "undefined" &&
        this.props.data.properties.context.indexOf("ui") !== -1) {
      var ui_state = this.props.app.isClientInList(
          this.props.app.state.fixtures, this.props.data);
      var ui_light_css = LIGHT_CSS_MAP[ui_state ? "light-toggle-on"
                                                : "light-toggle-off"];
      ui_span = (
        <span className={"label client-info-terminal " + ui_light_css}
            data-mid={this.props.data.key} onClick={this.onUIButtonClick}>
          UI
        </span>
      )
    }
    return (
      <div className="client-info">
        <div className="client-info-mid">
          {this.props.children}
        </div>
        <span className="label label-warning client-info-terminal"
            data-mid={this.props.data.key} onClick={this.openTerminal}>
          Terminal
        </span>
        {ui_span}
      </div>
    );
  }
});

var TerminalGroup = React.createClass({
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
      <div>
        <div className="terminal-group">
          {
            Object.keys(this.props.data).map(function (id) {
              var term = this.props.data[id];
              var extra = "";
              if (typeof(term.path) != "undefined") {
                extra = "?tty_device=" + term.path;
              }
              return (
                <TerminalWindow key={id} mid={term.mid} id={id} title={id}
                 path={"/api/agent/tty/" + term.mid + extra}
                 uploadPath={"/api/agent/upload/" + term.mid}
                 app={this.props.app} progressBars={this.refs.uploadProgress}
                 onControl={onControl} onCloseClicked={onCloseClicked} />
              );
            }.bind(this))
          }
        </div>
        <div className="upload-progress">
          <UploadProgress ref="uploadProgress" />
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
              <FixtureWindow key={item.mid} client={item}
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
  document.body
);
