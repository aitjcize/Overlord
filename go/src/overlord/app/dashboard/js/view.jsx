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
  loadClientsFromServer: function () {
    $.ajax({
      url: this.props.url,
      dataType: "json",
      success: function (data) {
        for (var i = 0; i < data.length; i++) {
          this.addClient(data[i], false);
        }
        this.filterClientList();
        this.forceUpdate();
      }.bind(this),
      error: function (xhr, status, err) {
        console.error(this.props.url, status, err.toString());
      }.bind(this)
    });
  },
  removeClientFromList: function (target_list, obj) {
    for (var i = 0; i < target_list.length; ++i) {
      if (target_list[i].mid == obj.mid) {
        target_list.splice(i, 1);
        break;
      }
    }
    return target_list;
  },
  isClientInList: function (target_list, client) {
    return target_list.filter(function (el, index, arr) {
      return el.mid == client.mid;
    }).length > 0;
  },
  fetchProperties: function (mid, callback) {
    var url = '/api/agent/properties/' + mid;
    $.ajax({
      url: url,
      dataType: "json",
      success: callback,
      error: function (xhr, status, err) {
        console.error(url, status, err.toString());
      }.bind(this)
    });
  },
  addClient: function(client, add_to_recent) {
    if (this.isClientInList(this.state.clients, client)) {
      return;
    }

    this.fetchProperties(client.mid, function(data) {
      client.properties = data;

      // Set status to hidden if the machine ID does not match search pattern
      if (typeof(this.lastPattern) != "undefined" &&
          !this.lastPattern.test(client.mid)) {
        client.status = "hidden";
      }

      if (this.isClientInList(this.state.clients, client)) {
        return;
      }
      this.state.clients.push(client);
      this.state.clients.sort(function(a, b) {
        return a.mid.localeCompare(b.mid);
      });

      if (add_to_recent) {
        // Add to recent client list
        // We are making a copy of client since we don't want to hide the
        // client in recentclient list (and Javascript is passed-by reference).
        client = JSON.parse(JSON.stringify(client))
        this.state.recentclients.splice(0, 0, client);
        this.state.recentclients = this.state.recentclients.slice(0, 5);
      }

      this.forceUpdate();
    }.bind(this));
  },
  addTerminal: function (id, term) {
    this.state.terminals[id] = term;
    this.forceUpdate();
  },
  addFixture: function (client) {
    if (this.isClientInList(this.state.fixtures, client)) {
      return;
    }
    this.state.fixtures.push(client);

    // compute how many fixtures we can put in the screen
    var screen = {
        width: window.innerWidth,
    };

    var sidebar = this.refs.sidebar.getDOMNode().getBoundingClientRect();

    screen.width -= sidebar.right;

    var nFixturePerRow = Math.floor(
       screen.width / (FIXTURE_WINDOW_WIDTH + FIXTURE_WINDOW_MARGIN * 2));
    nFixturePerRow = Math.max(1, nFixturePerRow);
    var nTotalFixture = Math.min(2 * nFixturePerRow, 8);

    // only keep recently opened @nTotalFixture fixtures.
    this.state.fixtures = this.state.fixtures.slice(-nTotalFixture);
    this.forceUpdate();
  },
  toggleFixtureState: function (client) {
    if (this.isClientInList(this.state.fixtures, client)) {
      this.removeFixture(client.mid);
    } else {
      this.addFixture(client);
    }
  },
  removeTerminal: function (id) {
    if (typeof(this.state.terminals[id]) != "undefined") {
      delete this.state.terminals[id];
    }
    this.forceUpdate();
  },
  removeFixture: function (id) {
    this.removeClientFromList(this.state.fixtures, {mid: id});
    this.forceUpdate();
  },
  filterClientList: function (val) {
    if (typeof(val) != "undefined") {
      this.lastPattern = new RegExp(val, "i");
    } else if (typeof(this.lastPattern) == "undefined") {
      this.lastPattern = new RegExp("", "i");
    }
    for (var i = 0; i < this.state.clients.length; i++) {
      if (!this.lastPattern.test(this.state.clients[i].mid)) {
        this.state.clients[i].status = "hidden";
      } else {
        this.state.clients[i].status = "";
      }
    }
    this.forceUpdate();
  },
  getInitialState: function () {
    return {clients: [], fixtures: [], recentclients: [], terminals: {}};
  },
  componentDidMount: function () {
    this.loadClientsFromServer();

    var socket = io(window.location.protocol + "//" + window.location.host,
                    {path: "/api/socket.io/"});
    this.socket = socket;

    socket.on("agent joined", function (msg) {
      var obj = JSON.parse(msg);
      this.addClient(obj, true);
    }.bind(this));

    socket.on("agent left", function (msg) {
      var obj = JSON.parse(msg);

      this.removeClientFromList(this.state.clients, obj);
      this.removeClientFromList(this.state.recentclients, obj);
      this.removeFixture(obj.mid);
      this.forceUpdate();
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
          <SideBar clients={this.state.clients} ref="sidebar"
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
    this.props.app.filterClientList(this.refs.filter.getDOMNode().value);
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
    var extra_class = "";
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
    if (typeof(this.props.data.status) != "undefined"
        && this.props.data.status == "hidden") {
      extra_class = "client-info-hidden";
    }
    return (
      <div className={"client-info " + extra_class}>
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
    var onClose = function (e) {
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
                 onControl={onControl} onClose={onClose} />
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

React.render(
  <App url="/api/agents/list" />,
  document.body
);
