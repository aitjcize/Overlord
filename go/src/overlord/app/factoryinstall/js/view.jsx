// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// View for Factory Install App
//
// Requires:
//   NavBar.jsx :: NavBar
//   TerminalWindow.jsx :: TerminalWindow
//
// - App
//  - NavBar
//  - ClientInfo
//    - TerminalWindow


var App = React.createClass({
  loadCookies: function (key, defaultValue) {
    return reactCookie.load(key) || defaultValue;
  },
  saveCookies: function (key, value) {
    // Set cookies expire 10 year later
    reactCookie.save(key, value, {maxAge: 10 * 365 * 86400});
  },
  loadClientsFromServer: function () {
    if (this.state.locked) {
      return;
    }
    $.ajax({
      url: this.props.url,
      dataType: "json",
      success: function (data) {
        if (!this.state.locked) {
          this.setState({clients: data});
        }
      }.bind(this),
      error: function (xhr, status, err) {
        console.error(this.props.url, status, err.toString());
      }.bind(this)
    });
  },
  removeClientFromList: function (target_list, obj) {
    for (var i = 0; i < target_list.length; ++i) {
      if (target_list[i].mid == obj.mid) {
        index = target_list[i].sids.indexOf(obj.sid);
        if (index != -1) {
          target_list[i].sids.splice(index, 1);
          if (!this.state.locked) {
            if (target_list[i].sids.length == 0) {
              target_list.splice(i, 1);
            }
          }
        }
        break;
      }
    }
  },
  onLockClicked: function (event) {
    this.setState(function (state, props) {
      return {locked: !state.locked};
    });
    this.saveCookies("locked", this.state.locked);
    if (this.state.locked) {
      var locked_mids = this.state.clients.map(function (event) {return event["mid"];});
      this.saveCookies("locked_mids", locked_mids);
    }
  },
  onTimeoutClicked: function (event) {
    $("#timeout-dialog").modal();
  },
  getTimeout: function (event) {
    return this.state.boot_timeout_secs;
  },
  onTimeoutDialogSaveClicked: function (event) {
    var new_timeout = Math.max(1, $("#boot_timeout_secs").val());
    this.setState({boot_timeout_secs: new_timeout});
    this.saveCookies("boot_timeout_secs", this.state.boot_timeout_secs);
  },
  onLayoutClicked: function (event) {
    $("#layout-dialog").modal();
  },
  onLayoutDialogSaveClicked: function (event) {
    var nrow = $("#nrow").val();
    if (nrow < 1) {
      nrow = 1;
    }
    var width = ($("#client-box-body").width() - 15) / nrow - 10;

    // Hack: the last stylesheet is the oldest one
    var st = document.styleSheets[document.styleSheets.length - 1];
    if (st.rules[0].selectorText == ".client-info") {
      st.removeRule(0);
    }
    st.insertRule(".client-info { width: " + width + " !important }", 0);
  },
  getInitialState: function () {
    var locked = this.loadCookies("locked", false);
    var clients = [];
    if (locked) {
      clients = this.loadCookies("locked_mids", []).map(
          function (mid) {return {mid: mid, sids: [], status: "disconnected"}});
    }
    return {
        locked: locked,
        clients: clients,
        boot_timeout_secs: this.loadCookies("boot_timeout_secs", 60)};
  },
  componentDidMount: function () {
    this.onLayoutDialogSaveClicked();
    this.loadClientsFromServer();
    setInterval(this.loadClientsFromServer, this.props.pollInterval);

    var socket = io(window.location.protocol + "//" + window.location.host,
                    {path: "/api/socket.io/"});
    socket.on("logcat joined", function (msg) {
      var obj = JSON.parse(msg);

      if (typeof(this.refs["client-" + obj.mid]) != "undefined") {
        this.refs["client-" + obj.mid].updateStatus("in-progress");
      }

      this.setState(function (state, props) {
        var client = state.clients.find(function (event, index, arr) {
          return event.mid == obj.mid;
        });
        if (typeof(client) == "undefined") {
          if (!state.locked) {
            state.clients.push({mid: obj.mid, sids: [obj.sid]});
          }
        } else {
          if (client.sids.indexOf(obj.sid) === -1) {
            client.sids.push(obj.sid);
          }
        }
      });
    }.bind(this));
    socket.on("logcat left", function (msg) {
      var obj = JSON.parse(msg);

      if (this.state.locked) {
        this.refs["client-" + obj.mid].updateStatus("disconnected");
      }
      this.setState(function (state, props) {
        this.removeClientFromList(state.clients, obj);
      });
    }.bind(this));
  },
  render: function() {
    var lock_btn_class = this.state.locked ? "btn-danger" : "btn-primary";
    var lock_btn_text = this.state.locked ? "Unlock" : "Lock";
    return (
      <div id="main">
        <NavBar name="Factory Install Dashboard" url="/api/apps/list" />
        <div className="client-box panel panel-info">
          <div className="panel-heading">
            Clients
            <div className="ctrl-btn-group">
              <button type="button" className="ctrl-btn btn btn-info"
                onClick={this.onLayoutClicked}>Layout</button>
              <button type="button" className="ctrl-btn btn btn-info"
                onClick={this.onTimeoutClicked}>Timeout</button>
              <button type="button" className={"ctrl-btn btn " + lock_btn_class}
                onClick={this.onLockClicked}>{lock_btn_text}</button>
            </div>
          </div>
          <div id="client-box-body" className="panel-body">
          {
            this.state.clients.map(function (item) {
              return (
                <ClientInfo key={item.mid} ref={"client-" + item.mid} data={item} root={this} getTimeout={this.getTimeout}>
                  {item.mid}
                </ClientInfo>
              );
            }.bind(this))
          }
          </div>
        </div>

        <div id="timeout-dialog" className="modal fade">
          <div className="modal-dialog">
            <div className="modal-content">
              <div className="modal-header">
                <button type="button" className="close" data-dismiss="modal" aria-label="Close">
                <span aria-hidden="true">&times;</span></button>
                <h4 className="modal-title">Timeout Settings</h4>
              </div>
              <div className="modal-body">
                Timeout seconds of the boot up:
                <input id="boot_timeout_secs" type="number" className="form-control" defaultValue={this.state.boot_timeout_secs} min="1"></input>
              </div>
              <div className="modal-footer">
                <button type="button" className="btn btn-default" data-dismiss="modal">Close</button>
                <button type="button" className="btn btn-primary" data-dismiss="modal"
                    onClick={this.onTimeoutDialogSaveClicked}>Save changes</button>
              </div>
            </div>
          </div>
        </div>
        <div id="layout-dialog" className="modal fade">
          <div className="modal-dialog">
            <div className="modal-content">
              <div className="modal-header">
                <button type="button" className="close" data-dismiss="modal" aria-label="Close">
                <span aria-hidden="true">&times;</span></button>
                <h4 className="modal-title">Layout Settings</h4>
              </div>
              <div className="modal-body">
                Number of device in a row:
                <input id="nrow" type="number" className="form-control" defaultValue="8" min="1"></input>
              </div>
              <div className="modal-footer">
                <button type="button" className="btn btn-default" data-dismiss="modal">Close</button>
                <button type="button" className="btn btn-primary" data-dismiss="modal"
                    onClick={this.onLayoutDialogSaveClicked}>Save changes</button>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }
});

var ClientInfo = React.createClass({
  getInitialState: function (event) {
    if (typeof(this.props.data.status) != "undefined") {
      return {status: this.props.data.status};
    }
    return {status: "in-progress"};
  },
  updateStatus: function (status) {
    this.setState({status: status});
  },
  onTagClick: function (event) {
    var sid = $(event.target).data("sid");
    $(this.refs["term-" + sid].getDOMNode()).css("display", "block");
  },
  onPanelClick: function (event) {
    // Workaround: crosbug.com/p/39839#11
    // It take too long from DUT power up to enter kernel. The operator clicks the panel after
    // power up the DUT, and shows error if the overlord doesn't connect the DUT in {boot_timeout_secs}.
    if (this.state.status == "disconnected" || this.state.status == "no-connection") {
      this.updateStatus("wait-connection");
      setTimeout(function () {
        if (this.state.status == "wait-connection") {
          this.updateStatus("no-connection");
        }
      }.bind(this), this.props.getTimeout() * 1000);
    }
  },
  render: function () {
    var statusClass = "panel-warning";
    if (this.state.status == "done") {
      statusClass = "panel-success";
    } else if (this.state.status == "error") {
      statusClass = "panel-danger";
    } else if (this.state.status == "disconnected") {
      statusClass = "panel-default";
    } else if (this.state.status == "wait-connection") {
      statusClass = "panel-warning";
    } else if (this.state.status == "no-connection") {
      statusClass = "panel-danger";
    }

    var message = "";
    if (this.state.status == "disconnected") {
      message = "Click after power up";
    } else if (this.state.status == "wait-connection") {
      message = "Waiting for connection";
    } else if (this.state.status == "no-connection") {
      message = "Failed. Power-cycle DUT and click again.";
    }

    var onError = function (event) {
      this.props.client.updateStatus("error");
    };

    var onMessage = function (data) {
      if (data.indexOf("Factory Installer Complete") != -1) {
        this.props.client.updateStatus("done");
      } else if (data.indexOf("\033[1;31m") != -1) {
        this.props.client.updateStatus("error");
      }
    };

    var onCloseClicked = function (event) {
      var el = document.getElementById(this.props.id);
      $(el).css("display", "none");
    };

    var mid = this.props.data.mid;
    return (
      <div className={"client-info panel " + statusClass} onClick={this.onPanelClick}>
        <div className="panel-heading">{this.props.children}</div>
        <div className="panel-body">
        {
          this.props.data.sids.map(function (sid) {
            return (
              <div className="client-info-tag-container">
                <div className="client-info-tag">
                  <span className="label label-warning client-info-terminal"
                      data-sid={sid} onClick={this.onTagClick}>
                    {sid}
                  </span>
                </div>
                <TerminalWindow key={sid} id={"terminal-" + mid + "-" + sid}
                 title={mid + " / " + sid}
                 path={"/api/log/" + mid + "/" + sid}
                 enableCopy={true}
                 onError={onError} onMessage={onMessage}
                 onCloseClicked={onCloseClicked} client={this}
                 ref={"term-" + sid} />
              </div>
            );
          }.bind(this))
        }
        {message}
        </div>
      </div>
    );
  }
});

ReactDOM.render(
  <App url="/api/logcats/list" pollInterval={60000} />,
  document.getElementById("body")
);
