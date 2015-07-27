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
  loadClientsFromServer: function () {
    if (this.state.locked) {
      return;
    }
    $.ajax({
      url: this.props.url,
      dataType: "json",
      success: function (data) {
        if (!this.state.locked) {
          this.state.clients = data;
          this.forceUpdate();
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
        index = target_list[i].cids.indexOf(obj.cid);
        if (index != -1) {
          target_list[i].cids.splice(index, 1);
          if (!this.state.locked) {
            if (target_list[i].cids.length == 0) {
              target_list.splice(i, 1);
            }
          }
        }
        break;
      }
    }
  },
  onLockClicked: function (e) {
    this.state.locked = $(e.target).attr('aria-pressed') == "true";
    this.forceUpdate();
  },
  onTimeoutClicked: function (e) {
    $('#timeout-dialog').modal();
  },
  getTimeout: function (e) {
    return this.state.boot_timeout_secs;
  },
  onTimeoutDialogSaveClicked: function (e) {
    this.state.boot_timeout_secs = Math.max(1, $('#boot_timeout_secs').val());
    this.forceUpdate();
  },
  onLayoutClicked: function (e) {
    $('#layout-dialog').modal();
  },
  onLayoutDialogSaveClicked: function (e) {
    var nrow = $('#nrow').val();
    if (nrow < 1) {
      nrow = 1;
    }
    var width = ($('#client-box-body').width() - 15) / nrow - 10;

    // Hack: the last stylesheet is the oldest one
    var st = document.styleSheets[document.styleSheets.length - 1];
    if (st.rules[0].selectorText == ".client-info") {
      st.removeRule(0);
    }
    st.insertRule(".client-info { width: " + width + " !important }", 0);
  },
  getInitialState: function () {
    return {clients: [], locked: false, boot_timeout_secs: 40};
  },
  componentDidMount: function () {
    this.onLayoutDialogSaveClicked();
    this.loadClientsFromServer();
    setInterval(this.loadClientsFromServer, this.props.pollInterval);

    var socket = io(window.location.protocol + "//" + window.location.host,
                    {path: "/api/socket.io/"});
    socket.on("logcat joined", function (msg) {
      var obj = JSON.parse(msg);
      var clients = this.state.clients;

      if (typeof(this.refs["client-" + obj.mid]) != "undefined") {
        this.refs["client-" + obj.mid].updateStatus("in-progress");
      }

      for (var i = 0; i < clients.length; i++) {
        if (clients[i].mid != obj.mid) {
          continue;
        }
        if (clients[i].cids.indexOf(obj.cid) == -1) {
          clients[i].cids.push(obj.cid);
        }
        this.forceUpdate();
        return
      }

      if (!this.state.locked) {
        this.state.clients.push({mid: obj.mid, cids: [obj.cid]});
      }
      this.forceUpdate();
    }.bind(this));
    socket.on("logcat left", function (msg) {
      var obj = JSON.parse(msg);

      if (this.state.locked) {
        this.refs["client-" + obj.mid].updateStatus("disconnected");
      }
      this.removeClientFromList(this.state.clients, obj);
      this.forceUpdate();
    }.bind(this));
  },
  render: function() {
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
              <button type="button" className="ctrl-btn btn btn-primary"
                data-toggle="button" onClick={this.onLockClicked}>Lock</button>
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
  getInitialState: function (e) {
    if (typeof(this.props.data.status) == "undefined") {
      return {status: this.props.data.status};
    }
    return {status: 'in-progress'};
  },
  updateStatus: function (status) {
    this.setState({status: status});
  },
  onTagClick: function (e) {
    var cid = $(e.target).data('cid');
    $(this.refs["term-" + cid].getDOMNode()).css('display', 'block');
  },
  onPanelClick: function (e) {
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

    var onError = function (e) {
      this.props.client.updateStatus("error");
    }

    var onMessage = function (msg) {
      var data = Base64.decode(msg.data);
      if (data.indexOf("Factory Installer Complete") != -1) {
        this.props.client.updateStatus("done");
      } else if (data.indexOf("\033[1;31m") != -1) {
        this.props.client.updateStatus("error");
      }
    };

    var onCloseClicked = function (e) {
      var el = document.getElementById(this.props.id);
      $(el).css("display", "none");
    }

    var mid = this.props.data.mid;
    return (
      <div className={"client-info panel " + statusClass} onClick={this.onPanelClick}>
        <div className="panel-heading">{this.props.children}</div>
        <div className="panel-body">
        {
          this.props.data.cids.map(function (cid) {
            return (
                <div className="client-info-tag">
                  <span className="label label-warning client-info-terminal"
                      data-cid={cid} onClick={this.onTagClick}>
                    {cid}
                  </span>
                  <TerminalWindow key={cid} id={"terminal-" + mid + "-" + cid}
                   title={mid + ' / ' + cid}
                   path={"/api/log/" + mid + "/" + cid}
                   onError={onError} onMessage={onMessage}
                   onCloseClicked={onCloseClicked} client={this}
                   ref={"term-" + cid} />
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

React.render(
  <App url="/api/logcats/list" pollInterval={60000} />,
  document.body
);
