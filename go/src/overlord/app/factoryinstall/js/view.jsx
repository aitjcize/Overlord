// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// Requires: common.jsx :: NavBar
//
// - App
//  - NavBar
//  - ClientInfo
//    - TerminalWindow

window.locked = false;

var App = React.createClass({
  loadClientsFromServer: function () {
    if (window.locked) {
      return
    }
    $.ajax({
      url: this.props.url,
      dataType: "json",
      success: function (data) {
        if (!window.locked) {
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
          if (!window.locked) {
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
    window.locked = $(e.target).attr('aria-pressed') == "true";
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
    return {clients: []};
  },
  componentDidMount: function () {
    this.onLayoutDialogSaveClicked();
    this.loadClientsFromServer();
    setInterval(this.loadClientsFromServer, this.props.pollInterval);

    var $this = this;
    var socket = io("http://" + window.location.host, {path: "/api/socket.io/"});
    socket.on("logcat joined", function (msg) {
      var obj = JSON.parse(msg);
      var clients = $this.state.clients;

      if (typeof($this.refs["client-" + obj.mid]) != "undefined") {
        $this.refs["client-" + obj.mid].updateStatus("in-progress");
      }

      for (var i = 0; i < clients.length; i++) {
        if (clients[i].mid != obj.mid) {
          continue;
        }
        if (clients[i].cids.indexOf(obj.cid) == -1) {
          clients[i].cids.push(obj.cid);
        }
        $this.forceUpdate();
        return
      }

      if (!window.locked) {
        $this.state.clients.push({mid: obj.mid, cids: [obj.cid]});
      }
      $this.forceUpdate();
    });
    socket.on("logcat left", function (msg) {
      var obj = JSON.parse(msg);

      if (window.locked) {
        $this.refs["client-" + obj.mid].updateStatus("disconnected");
      }
      $this.removeClientFromList($this.state.clients, obj);
      $this.forceUpdate();
    });
  },
  render: function() {
    var $this=this;
    return (
      <div id="main">
        <NavBar name="Factory Install Dashboard" url="/api/apps/list" />
        <div className="client-box panel panel-info">
          <div className="panel-heading">
            Clients
            <div className="ctrl-btn-group">
              <button type="button" className="ctrl-btn btn btn-info"
                onClick={this.onLayoutClicked}>Layout</button>
              <button type="button" className="ctrl-btn btn btn-primary"
                data-toggle="button" onClick={this.onLockClicked}>Lock</button>
            </div>
          </div>
          <div id="client-box-body" className="panel-body">
          {
            this.state.clients.map(function (item) {
              return (
                <ClientInfo key={item.mid} ref={"client-" + item.mid} data={item} root={$this}>
                  {item.mid}
                </ClientInfo>
              );
            })
          }
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
  onClick: function (e) {
    var cid = $(e.target).data('cid');
    $(this.refs["term-" + cid].getDOMNode()).css('display', 'block');
  },
  render: function () {
    var statusClass = "panel-warning";
    if (this.state.status == "done") {
      statusClass = "panel-success";
    } else if (this.state.status == "error") {
      statusClass = "panel-danger";
    } else if (this.state.status == "disconnected") {
      statusClass = "panel-default";
    }
    var $this = this;
    var mid = this.props.data.mid;
    return (
      <div className={"client-info panel " + statusClass}>
        <div className="panel-heading">{this.props.children}</div>
        <div className="panel-body">
        {
          this.props.data.cids.map(function (cid) {
            var data = {mid: mid, cid: cid};
            return (
                <div className="client-info-tag">
                  <span className="label label-warning client-info-terminal"
                      data-cid={cid} onClick={$this.onClick}>
                    {cid}
                  </span>
                  <TerminalWindow key={cid} data={data}
                      updateStatus={$this.updateStatus} ref={"term-" + cid} />
                </div>
            );
          })
        }
        </div>
      </div>
    );
  }
});

var TerminalWindow = React.createClass({
  getInitialState: function () {
    return {"pages": []};
  },
  componentDidMount: function () {
    var mid = this.props.data.mid;
    var cid = this.props.data.cid;
    var el = document.getElementById("terminal-" + mid + "-" + cid);
    var url = "ws://" + window.location.host + "/api/log/" + mid + "/" + cid;
    var sock = new WebSocket(url);

    var $el = $(el);
    var $this = this;

    this.sock = sock;
    this.el = el;

    sock.onerror = function (e) {
      console.log("socket error", e);
      $this.props.updateStatus("error");
    };

    $el.draggable({
      // Once the window is dragged, make it position fixed.
      stop: function() {
        offsets = el.getBoundingClientRect();
        $el.css({
          position: 'fixed',
          top: offsets.top+"px",
          left: offsets.left+"px"
        });
      },
      cancel: ".terminal"
    });
    sock.onclose = function (e) {
      $this.props.updateStatus("disconnected");
    }
    sock.onopen = function (e) {
      var term = new Terminal({
        cols: 80,
        rows: 24,
        useStyle: true,
        screenKeys: true
      });

      term.open(el);

      term.on('title', function(title) {
        $el.find('.terminal-title').text(title);
      });

      term.on('data', function(data) {
        sock.send(data);
      });

      sock.onmessage = function(msg) {
        var data = Base64.decode(msg.data);
        if (data.indexOf("Factory Installer Complete") != -1) {
          $this.props.updateStatus("done");
        } else if (data.indexOf("\033[1;31m") != -1) {
          $this.props.updateStatus("error");
        }
        term.write(data);
      };
    };
  },
  onWindowMouseDown: function(e) {
    if (typeof(window.maxz) == "undefined") {
      window.maxz = 100;
    }
    var $el = $(this.el);
    if ($el.css("z-index") != window.maxz) {
      window.maxz += 1;
      $el.css("z-index", window.maxz);
    }
  },
  onCloseMouseUp: function(e) {
    $(this.el).css('display', 'none');
  },
  render: function () {
    return (
      <div className="terminal-window"
          id={"terminal-" + this.props.data.mid + "-" + this.props.data.cid}
          onMouseDown={this.onWindowMouseDown}>
        <div className="terminal-title">{this.props.data.mid}</div>
        <div className="terminal-control">
          <div className="terminal-close" onMouseUp={this.onCloseMouseUp}></div>
        </div>
      </div>
    );
  }
});

React.render(
  <App url="/api/logcats/list" pollInterval={60000} />,
  document.body
);
