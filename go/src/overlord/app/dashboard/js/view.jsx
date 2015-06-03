// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// View for Dashboard App
//
// Requires:
//   NavBar.jsx :: NavBar
//   TerminalWindow.jsx :: TerminalWindow
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
//  - TerminalGroup
//    - TerminalWindow


function abbr(str, len) {
  if (str.length > len) {
    return str.substr(0, len - 3) + "...";
  }
  return str
}

var App = React.createClass({
  loadClientsFromServer: function () {
    $.ajax({
      url: this.props.url,
      dataType: "json",
      success: function (data) {
        this.state.clients = data;
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
  addTerminal: function (data) {
    found = this.state.terminals.filter(function (el, index, arr) {
      return el.mid == data.mid;
    })
    if (found.length != 0) {
      return;
    }
    this.state.terminals.push(data);
    this.forceUpdate();
  },
  removeTerminal: function (mid) {
    this.removeClientFromList(this.state.terminals, {mid: mid});
    this.forceUpdate();
  },
  filterClientList: function (val) {
    var pattern = new RegExp(val);
    for (var i = 0; i < this.state.clients.length; i++) {
      if (!pattern.test(this.state.clients[i].mid)) {
        this.state.clients[i].status = "hidden";
      } else {
        this.state.clients[i].status = "";
      }
    }
    this.forceUpdate();
  },
  getInitialState: function () {
    return {clients: [], recentclients: [], terminals: []};
  },
  componentDidMount: function () {
    this.loadClientsFromServer();
    setInterval(this.loadClientsFromServer, this.props.pollInterval);

    var socket = io(window.location.protocol + "//" + window.location.host,
                    {path: "/api/socket.io/"});

    socket.on("agent joined", function (msg) {
      var obj = JSON.parse(msg)
      this.state.recentclients.splice(0, 0, obj);
      this.state.recentclients = this.state.recentclients.slice(0, 5);
      this.state.clients.push(obj);
      this.forceUpdate();
    }.bind(this));
    socket.on("agent left", function (msg) {
      var obj = JSON.parse(msg);

      this.removeClientFromList(this.state.clients, obj);
      this.removeClientFromList(this.state.recentclients, obj);
      this.removeClientFromList(this.state.terminals, obj);
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
        <NavBar name="Dashboard" url="/api/apps/list" />
        <SideBar clients={this.state.clients}
            recentclients={this.state.recentclients} app={this} />
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
  onClick: function (e) {
    this.props.app.addTerminal(this.props.data);
  },
  render: function () {
    var display = "block";
    var extra_class = "";
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
            data-mid={this.props.data.key} onClick={this.onClick}>
          Terminal
        </span>
      </div>
    );
  }
});

var TerminalGroup = React.createClass({
  render: function () {
    var onClose = function (e) {
      this.props.app.removeTerminal(this.props.mid);
    }
    return (
      <div className="terminal-group">
        {
          this.props.data.map(function (item) {
            return (
              <TerminalWindow key={item.mid} mid={item.mid}
               id={"terminal-" + item.mid} title={item.mid}
               path={"/api/agent/pty/" + item.mid}
               app={this.props.app} onClose={onClose} />
            );
          }.bind(this))
        }
      </div>
    );
  }
});

React.render(
  <App url="/api/agents/list" pollInterval={60000} />,
  document.body
);
