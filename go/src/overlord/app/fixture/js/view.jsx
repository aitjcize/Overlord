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
  mixins: [BaseApp],
  onNewClient: function (client) {
    return !(typeof(client.properties) == "undefined" ||
        typeof(client.properties.context) == "undefined" ||
        client.properties.context.indexOf("ui") === -1);
  },
  addTerminal: function (id, term) {
    this.state.terminals[id] = term;
    this.setState({terminals: this.state.terminals});
  },
  removeTerminal: function (id) {
    if (typeof(this.state.terminals[id]) != "undefined") {
      delete this.state.terminals[id];
    }
    this.setState({terminals: this.state.terminals});
  },
  getInitialState: function () {
    return {terminals: {}};
  },
  componentWillMount: function () {
    this.addOnNewClientHandler(this.onNewClient);
  },
  componentDidMount: function () {
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
    // compute how many clients we can put in the screen
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
    this.props.app.setMidFilterPattern(this.refs.filter.value);
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
                      <li key={i} {...extra}>
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

ReactDOM.render(
  <App url="/api/agents/list"/>,
  document.body
);
