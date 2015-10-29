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
        data.properties.context.indexOf("ui") === -1) {
      return;
    }
    this.state.fixtures.push(data);
    this.state.fixtures.sort(function (a, b) {
      return a.mid.localeCompare(b.mid);
    });
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
  setFilterPattern: function (pattern) {
    if (typeof(pattern) != "undefined") {
      this.lastPattern = new RegExp(pattern, "i");
    } else if (typeof(this.lastPattern) == "undefined") {
      this.lastPattern = new RegExp("", "i");
    }
    this.forceUpdate();
  },
  getFilteredClientList: function () {
    if (typeof(this.lastPattern) != "undefined") {
      var filteredList = [];
      for (var i = 0; i < this.state.fixtures.length; i++) {
        if (this.lastPattern.test(this.state.fixtures[i].mid)) {
          filteredList.push(this.state.fixtures[i]);
        }
      }
      return filteredList;
    } else {
      return this.state.fixtures.slice();
    }
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
  computePageSize: function () {
    // compute how many fixtures we can put in the screen
    var screen = {
        width: window.innerWidth,
    };

    var nFixturePerRow = Math.floor(
       screen.width / (FIXTURE_WINDOW_WIDTH + FIXTURE_WINDOW_MARGIN * 2));
    nFixturePerRow = Math.max(1, nFixturePerRow);
    var nTotalFixture = Math.min(2 * nFixturePerRow, 8);
    return nTotalFixture;
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
                 onControl={onControl} onCloseClicked={onCloseClicked} />
              );
            }.bind(this))
          }
        </div>
        <div className="upload-progress">
          <UploadProgress ref="uploadProgress" />
        </div>
        <Paginator header="Clients" app={this}
            pageSize={this.computePageSize()}>
          {
            this.getFilteredClientList().map(function (data) {
              return (
                <FixtureWindow key={data.mid} client={data} app={this}/>
              );
            }.bind(this))
          }
        </Paginator>
      </div>
    );
  }
});

Paginator = React.createClass({
  onKeyUp: function (e) {
    this.props.app.setFilterPattern(this.refs.filter.getDOMNode().value);
  },
  changePage: function (i) {
    this.setState({pageNumber: i});
  },
  getInitialState: function () {
    return {pageNumber: 0};
  },
  render: function () {
    var nPage = Math.ceil(this.props.children.length / this.props.pageSize);
    var pageNumber = Math.max(Math.min(this.state.pageNumber, nPage - 1), 0);
    var start = pageNumber * this.props.pageSize;
    var end = start + this.props.pageSize;
    var children = this.props.children.slice(start, end);

    var pages = Array.apply(null, {length: nPage}).map(Number.call, Number);

    return (
      <div className="app-box panel panel-info">
        <div className="panel-heading">
          <div className="container-fluid panel-container">
            <div className="col-xs-3 text-left">
              <h2 className="panel-title">{this.props.header}</h2>
            </div>
            <div className="col-xs-6 text-center">
              <ul className="pagination panel-pagination">
                <li>
                  <a href="#" aria-label="Previous"
                      onClick={this.changePage.bind(this, pageNumber - 1)}>
                    <span aria-hidden="true">&laquo;</span>
                  </a>
                </li>
                {
                  pages.map(function (i) {
                    var extra = {};
                    if (i == pageNumber) {
                        extra.className = "active";
                    }
                    return (
                      <li {...extra}>
                        <a onClick={this.changePage.bind(this, i)} href="#">
                          {i + 1}
                        </a>
                      </li>
                    )
                  }.bind(this))
                }
                <li>
                  <a href="#" aria-label="Next"
                      onClick={this.changePage.bind(this, pageNumber + 1)}>
                    <span aria-hidden="true">&raquo;</span>
                  </a>
                </li>
              </ul>
            </div>
            <div className="col-xs-3">
              <div className="col-xs-6 pull-right">
              <input type="text" ref="filter" placeholder="keyword"
                  className="filter-input form-control"
                  onKeyUp={this.onKeyUp} />
              </div>
            </div>
          </div>
        </div>
        <div className="panel-body">
        {
          children.map(function (child) {
            return child;
          }.bind(this))
        }
        </div>
      </div>
    );
  }
});

React.render(
  <App url="/api/agents/list"/>,
  document.body
);
