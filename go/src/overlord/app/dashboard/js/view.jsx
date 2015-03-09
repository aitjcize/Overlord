// - App
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

    var $this = this;
    var socket = io("http://" + window.location.host, {path: "/api/socket.io/"});
    socket.on("agent joined", function (msg) {
      var obj = JSON.parse(msg)
      $this.state.recentclients.splice(0, 0, obj);
      $this.state.recentclients = $this.state.recentclients.slice(0, 5);
      $this.state.clients.push(obj);
      $this.forceUpdate();
    });
    socket.on("agent left", function (msg) {
      var obj = JSON.parse(msg);

      $this.removeClientFromList($this.state.clients, obj);
      $this.removeClientFromList($this.state.recentclients, obj);
      $this.removeClientFromList($this.state.terminals, obj);
      $this.forceUpdate();
    });
  },
  render: function() {
    return (
      <div id="main">
        <h1 className="text-center">Overlord Dashboard</h1>
        <SideBar clients={this.state.clients}
            recentclients={this.state.recentclients} root={this} />
        <TerminalGroup data={this.state.terminals} root={this} />
      </div>
    );
  }
});

var SideBar = React.createClass({
  render: function () {
    return (
      <div className="sidebar">
        <ClientBox data={this.props.clients} root={this.props.root} />
        <RecentList data={this.props.recentclients} root={this.props.root} />
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
          <FilterInput root={this.props.root} />
          <ClientList data={this.props.data} root={this.props.root} />
        </div>
      </div>
    );
  }
})

var FilterInput = React.createClass({
  onKeyUp: function (e) {
    this.props.root.filterClientList(this.refs.filter.getDOMNode().value);
  },
  render: function() {
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
    var $this = this;
    return (
      <div className="list-box client-list">
        {
          this.props.data.map(function (item) {
            return (
              <ClientInfo key={item.mid} data={item} root={$this.props.root}>
                {abbr(item.mid, 36)}
              </ClientInfo>
              );
          })
        }
      </div>
    );
  }
});

var RecentList = React.createClass({
  render: function () {
    var $this = this;
    return (
      <div className="recent-box panel panel-info">
        <div className="panel-heading">Recent Connected Clients</div>
        <div className="panel-body">
          <div className="list-box recent-list">
            {
              this.props.data.map(function (item) {
                return (
                  <ClientInfo key={item.mid} data={item} root={$this.props.root}>
                    {abbr(item.mid, 36)}
                  </ClientInfo>
                  );
              })
            }
          </div>
        </div>
      </div>
    )
  }
});

var ClientInfo = React.createClass({
  onClick: function(e) {
    this.props.root.addTerminal(this.props.data);
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
    var $this = this;
    return (
      <div className="terminal-group">
        {
          this.props.data.map(function (item) {
            return (
              <TerminalWindow key={item.mid} data={item} root={$this.props.root} />
            );
          })
        }
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
    var el = document.getElementById("terminal-" + mid);
    var $el = $(el);
    var ws_url = "ws://" + window.location.host + "/api/agent/pty/" + mid;
    var sock = new WebSocket(ws_url);
    this.sock = sock;

    sock.onerror = function (e) {
      console.log("socket error", e);
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
        term.write(Base64.decode(msg.data));
      };
    };
  },
  onWindowMouseDown: function(e) {
    if (typeof(window.maxz) == "undefined") {
      window.maxz = 100;
    }
    var $el = $(e.target).parents('.terminal-window');
    if ($el.css("z-index") != window.maxz) {
      window.maxz += 1;
      $el.css("z-index", window.maxz);
    }
  },
  onCloseMouseUp: function(e) {
    this.props.root.removeTerminal(this.props.data.mid);
    this.sock.close();
  },
  render: function () {
    return (
      <div className="terminal-window" id={"terminal-" + this.props.data.mid}
          onMouseDown={this.onWindowMouseDown}>
        <div className="terminal-title">{abbr(this.props.data.mid, 90)}</div>
        <div className="terminal-control">
          <div className="terminal-close" onMouseUp={this.onCloseMouseUp}></div>
        </div>
      </div>
    );
  }
});

React.render(
  <App url="/api/agents/list" pollInterval={60000} />,
  document.body
);
