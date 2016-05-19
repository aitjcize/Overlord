// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// External dependencies:
// - JsRender: https://github.com/BorisMoore/jsrender
//
// Internal dependencies:
// - UploadProgressWidget
//
// View for FixtureWidget:
// - FixtureWidget
//   props:
//     app: react reference to the app object with addTerminal method
//     client: a overlord client client object with properties object
//     progressBars: an react reference to UploadProgressWidget instance
//  - Display
//  - Lights
//  - Terminals
//  - Controls
//  - MainLog
//  - AuxLogs
//   - AuxLog

var LOG_BUF_SIZE = 8192;

var FIXTURE_WINDOW_WIDTH = 420;
var FIXTURE_WINDOW_MARGIN = 10;

var LIGHT_CSS_MAP = {
  'light-toggle-off': 'label-danger',
  'light-toggle-on': 'label-success'
};

// FixtureWidget defines the layout and behavior of a fixture window,
// which has display, lights, terminals, controls and logs.
//
// Usage:
// A terminal description object would looks like the following in json:
// {
//   "name":"NUC",
//   "mid":"ghost 1"
//   // @path attribute is optional, without @path, it means that we are
//   // connecting to the fixture itself.
//   "path": "some path"
// }
// Given @id as identifier, and @term as a terminal description object, to open
// a terminal connection, you can use TerminalWindow:
//   <TerminalWindow key={id} mid={term.mid} id={id} title={id}
//       path={"/api/agent/tty/" + term.mid + extra}
//       uploadPath={"/api/agent/upload/" + term.mid}
//       app={this.props.app} progressBars={this.refs.uploadProgress}
//       onControl={onControl} onClose={onClose} />
//   where @extra = "?tty_device=" + term.path if term.path is defined.
//
// A client object would looks like the following in json:
// {
//   "mid": "machine ID",
//   "sid": "serial ID",
//   // see properties.sample.json
//   "properties": {
//     "ip": "127.0.0.1",
//     "ui": {
//       // A master command which updates ui states.
//       // "update_ui_status" is a script we wrote that will respect
//       // @init_cmd and @poll attributes in lights and display.data, you can
//       // implement your own script instead.
//       "update_ui_command": "update_ui_status",
//       // Lights are used to show current status of the fixture, lights has
//       // two states: on and off, which is represent by setting "light"
//       // attribute to 'light-toggle-on' or 'light-toggle-off' (see below)
//       "lights": [
//         {
//           // Identifier of this light, if the output of @command contains
//           // LIGHT[@id]='light-toggle-on', then @light will be set to on.
//           "id": "ccd",
//           // Text to be shown
//           "label": "CCD",
//           // Set default state to off
//           "light": "light-toggle-off",
//           // Command to execute when clicked
//           "command": "case_close_debug",
//           // Will be called when the FixtureWidget is opened.
//           "init_cmd": "case_close_debug status"
//         },
//         {
//           "id": "dut-lid",
//           "label": "DUT LID"
//           "light": "light-toggle-off",
//           // @cmd will be execute every @interval milliseconds, you can
//           // output LIGHT[@id]='light-toggle-on' to change the light.
//           "poll": {
//             "cmd": "check_dut_exists -t lid",
//             "interval": 20000
//           },
//         }, ...
//       ],
//       // A list of terminals connected to this fixture, for example, there
//       // might be a terminal for fixture itself and a terminal for DUT.
//       "terminals": [
//         // Without @path_cmd attribute, will connect to fixture itself.
//         {
//           "name": "NUC"
//         },
//         // @path_cmd will be used to get the path of device.
//         {
//           "name": "AP"
//           "path_cmd": "ls /dev/google/Ryu_debug-*/serial/AP 2>/dev/null",
//         },
//       ],
//       // A display section
//       "display": {
//         // A jsrender template
//         "template": "<b>Report</b><ul><li>Version: {{:version}}</li>"
//                     "<li>Status: {{:status}}</li></ul>",
//         "data": [
//           {
//             // id: the name of data binding in the template
//             "id": "version",
//             // Will be called when the FixtureWidget is opened.
//             "init_cmd": "get_version",
//           },
//           {
//             "id": "status",
//             // @cmd will be execute every @interval milliseconds, you can
//             // output DATA[@id]='value' to change the binding value.
//             "poll": {
//               "cmd": "get_status",
//               "interval": 20000
//             },
//           }, ...
//         ]
//       },
//       // A list of buttons to control some functionality of the fixture.
//       "controls": [
//         // A command
//         {
//           "name": "Factory Restart"
//           "command": "factory_restart",
//         },
//         // A command that will be toggled between two state.
//         {
//           "name": "Voltage Measurement",
//           "type": "toggle",
//           "on_command": "command to start measuring voltage",
//           "off_command": "command to stop measuring"
//         },
//         // A button that allow uploading a file, then execute a command
//         {
//           "name": "Update Tollkit",
//           "type": "upload",
//           "dest": "/tmp/install_factory_toolkit.run",
//           // @command is optional, you can omit this if you don't need to
//           // execute any command.
//           "command": "rm -rf /usr/local/factory && "
//                      "sh /tmp/install_factory_toolkit.run -- -y &&"
//                      "factory_restart"
//         },
//         // A button that allow execute a command, then download a file
//         {
//           "name": "Download Log",
//           "type": "download",
//           // @command is optional, you can omit this if you don't need to
//           // execute any command.
//           "command": "dmesg > /tmp/dmesg.log",
//
//           // The filename can be specified in both static @filename
//           // attribute, or a @filename_cmd attribute, which the output
//           // of the command is the download filename.
//           "filename": "/tmp/dmesg.log",
//           // or (exclusively)
//           "filename_cmd": "get_filename_cmd",
//         },
//         // A button that opens a link
//         {
//           "name": "VNC",
//           "type": "link",
//           // @url is a URL template, here is a list of supported attributes:
//           // host: the hostname of the webserver serving this page
//           // port: the HTTP port of the webserver serving this page
//           // client: the client object
//           "url": "/third_party/noVNC/vnc_auto.html?host={{:host}}&"
//                  "port={{:port}}&path=api/agent/forward/{{:client.mid}}"
//                  "%3fport=5901"
//         },
//         // A group of commands
//         {
//           "name": "Fixture control"
//           "group": [
//             {
//               "name": "whale close"
//               "command": "whale close",
//             },
//             {
//               "name": "whale open"
//               "command": "whale open",
//             },
//             {
//               "name": "io insertion"
//               "command": "whale insert",
//             },
//             {
//               "name": "charging"
//               "command": "whale charge",
//             }
//           ],
//         }
//       ],
//       // Path to the log files, FixtureWidget will keep polling the latest
//       // content of these file.
//       "logs": [
//         "/var/log/factory.log", ...
//       ]
//     },
//     // What catagories this fixture belongs to. If it contains "ui", an "UI"
//     // button will be shown on the /dashboard page. If it contains "whale",
//     // it will be shown on the /whale page.
//     "context": [
//       "ui", "whale", ...
//     ]
//   },
// }
var FixtureWidget = React.createClass({
  executeRemoteCmd: function (mid, cmd) {
    if (!this.isMounted()) {
      return;
    }
    var url = "ws" + ((window.location.protocol == "https:")? "s": "" ) +
              "://" + window.location.host + "/api/agent/shell/" + mid +
              "?command=" + encodeURIComponent(cmd);
    var sock = new WebSocket(url);
    var deferred = $.Deferred();

    sock.onopen = function (event) {
      sock.onmessage = function (msg) {
        if (!this.isMounted()) {
          sock.close();
          return;
        }
        if (msg.data instanceof Blob) {
          ReadBlobAsText(msg.data, function(text) {
            this.refs.mainlog.appendLog(text);
          }.bind(this));
        }
      }.bind(this)
    }.bind(this)

    sock.onclose = function (event) {
      deferred.resolve();
    }
    this.socks.push(sock);

    return deferred.promise();
  },
  extractUIMessages: function (text) {
    if (typeof(this.refs.lights) != "undefined") {
      text = this.refs.lights.extractLightMessages(text);
    }
    if (typeof(this.refs.display) != "undefined") {
      text = this.refs.display.extractDataMessages(text);
    }
    return text;
  },
  componentDidMount: function () {
    var client = this.props.client;
    var update_ui_command = client.properties.ui.update_ui_command;
    setTimeout(function() {
      this.executeRemoteCmd(client.mid, update_ui_command);
    }.bind(this), 1000);
  },
  componentWillUnmount: function () {
    for (var i = 0; i < this.socks.length; ++i) {
      this.socks[i].close();
    }
  },
  getInitialState: function () {
    this.socks = [];
    return {};
  },
  render: function () {
    var client = this.props.client;
    var ui = client.properties.ui;
    var style = {
      width: FIXTURE_WINDOW_WIDTH + 'px',
      margin: FIXTURE_WINDOW_MARGIN + 'px',
    };
    var display = ui.display && (
          <Display ref="display" client={client} fixture={this} />
        ) || "";
    var lights = ui.lights && (
          <Lights ref="lights" client={client} fixture={this} />
        ) || "";
    var terminals = ui.terminals && (
          <Terminals client={client} app={this.props.app} fixture={this} />
        ) || "";
    var controls = ui.controls && (
          <Controls ref="controls" client={client} fixture={this}
           progressBars={this.props.progressBars} />
        ) || "";
    var auxlogs = ui.logs && (
          <AuxLogs client={client} fixture={this} />
        ) || "";
    return (
      <div className="fixture-block panel panel-success" style={style}>
        <div className="panel-heading text-center">{abbr(client.mid, 60)}</div>
        <div className="panel-body">
          {display}
          {lights}
          {terminals}
          {controls}
          <MainLog ref="mainlog" fixture={this} id={client.mid} />
          {auxlogs}
        </div>
      </div>
    );
  }
});

var Display = React.createClass({
  updateDisplay: function (key, value) {
    this.setState(function (state, props) {
      state[key] = value;
    });
  },
  extractDataMessages: function (msg) {
    var patt = /DATA\[(.*?)\]\s*=\s*'(.*?)'\n?/g;
    var found;
    while (found = patt.exec(msg)) {
      this.updateDisplay(found[1], found[2]);
    }
    return msg.replace(patt, "");
  },
  getInitialState: function () {
    var display = this.props.client.properties.ui.display;
    var data = {};
    for (var i = 0; i < display.data.length; i++) {
      data[display.data[i].id] = ""
    }
    return data;
  },
  componentWillMount: function() {
    var display = this.props.client.properties.ui.display;
    this.template = $.templates(display.template);
  },
  render: function () {
    var client = this.props.client;
    var displayHTML = this.template.render(this.state);
    return (
      <div className="status-block well well-sm">
        <div dangerouslySetInnerHTML={{__html: displayHTML}} />
      </div>
    );
  }
});

var Lights = React.createClass({
  updateLightStatus: function (id, status_class) {
    var node = $(this.refs[id]);
    node.removeClass(this.refs[id].props.prevLight);
    node.addClass(status_class);
    this.refs[id].props.prevLight = status_class;
  },
  extractLightMessages: function (msg) {
    var patt = /LIGHT\[(.*?)\]\s*=\s*'(.*?)'\n?/g;
    var found;
    while (found = patt.exec(msg)) {
      this.updateLightStatus(found[1], LIGHT_CSS_MAP[found[2]]);
    }
    return msg.replace(patt, "");
  },
  render: function () {
    var client = this.props.client;
    var lights = client.properties.ui.lights || [];
    return (
      <div className="status-block well well-sm">
      {
        lights.map(function (light) {
          var extra_css = "";
          var extra = {};
          if (typeof(light.command) != "undefined") {
            extra_css = "status-light-clickable";
            extra.onClick = function() {
              this.props.fixture.executeRemoteCmd(client.mid, light.command);
            }.bind(this);
          }
          var light_css = LIGHT_CSS_MAP[light.light];
          return (
            <span key={light.id} className={"label " + extra_css + " " +
              light_css} prevLight={light_css} ref={light.id} {...extra}>
              {light.label}
            </span>
          );
        }.bind(this))
      }
      </div>
    );
  }
});

var Terminals = React.createClass({
  onTerminalClick: function (event) {
    var target = $(event.target);
    var mid = target.data("mid");
    var term = target.data("term");
    var id = mid + "::" + term.name;

    // Add mid reference to term object
    term.mid = mid;

    if (typeof(term.path_cmd) != "undefined" &&
        term.path_cmd.match(/^\s+$/) == null) {
      getRemoteCmdOutput(mid, term.path_cmd).done(function (path) {
        if (path.replace(/^\s+|\s+$/g, "") == "") {
          alert("This TTY device does not exist!");
        } else {
          term.path = path;
          this.props.app.addTerminal(id, term);
        }
      }.bind(this));
      return;
    }

    this.props.app.addTerminal(id, term);
  },
  render: function () {
    var client = this.props.client;
    var terminals = client.properties.ui.terminals || [];
    return (
      <div className="status-block well well-sm">
      {
        terminals.map(function (term) {
          return (
            <button className="btn btn-xs btn-info" data-mid={client.mid}
                data-term={JSON.stringify(term)} key={term.name}
                onClick={this.onTerminalClick}>
            {term.name}
            </button>
          );
        }.bind(this))
      }
      </div>
    );
  }
});

var Controls = React.createClass({
  onCommandClicked: function (event) {
    var target = $(event.target);
    var ctrl = target.data("ctrl");
    var mid = target.data("mid");
    var fixture = this.props.fixture;

    if (ctrl.type == "toggle") {
      if (target.hasClass("active")) {
        fixture.executeRemoteCmd(mid, ctrl.off_command);
        target.removeClass("active");
      } else {
        fixture.executeRemoteCmd(mid, ctrl.on_command);
        target.addClass("active");
      }
    } else if (ctrl.type == "download") {
      // Helper function for downloading a file
      var downloadFile = function (filename) {
        var url = window.location.protocol + "//" + window.location.host +
                  "/api/agent/download/" + mid +
                  "?filename=" + filename;
        $("<iframe src='" + url + "' style='display:none'>" +
          "</iframe>").appendTo('body');
      }
      var startDownload = function () {
        // Check if there is filename_cmd
        if (typeof(ctrl.filename_cmd) != "undefined") {
          getRemoteCmdOutput(mid, ctrl.filename_cmd)
            .done(function (path) { downloadFile(path); });
        } else {
          downloadFile(ctrl.filename);
        }
      }
      if (typeof(ctrl.command) != "undefined") {
        fixture.executeRemoteCmd(mid, ctrl.command)
          .done(function() { startDownload(); });
      } else {
        startDownload();
      }
    } else {
      fixture.executeRemoteCmd(mid, ctrl.command);
    }
  },
  onUploadButtonChanged: function (event) {
    var file = event.target;
    var mid = $(file).data("mid");
    var ctrl = $(file).data("ctrl");

    var runCommand = function () {
      if (typeof(ctrl.command) != "undefined") {
        this.props.fixture.executeRemoteCmd(mid, ctrl.command);
      }
      // Reset the file value, so user can click the button again.
      file.value = "";
    };

    if (file.value != "") {
      this.props.progressBars.upload("/api/agent/upload/" + mid,
                                     file.files[0], ctrl.dest,
                                     undefined, runCommand.bind(this));
    }
  },
  componentDidMount: function () {
    $('input[type=file]').fileinput();
  },
  render: function () {
    var client = this.props.client;
    var mid = client.mid;
    var controls = client.properties.ui.controls || [];
    var btnClasses = "btn btn-xs btn-primary";
    return (
      <div className="controls-block well well-sm">
      {
        controls.map(function (control) {
          if (typeof(control.group) != "undefined") { // sub-group
            return (
              <div className="well well-sm well-inner" key={control.name}>
              {control.name}<br />
              {
                control.group.map(function (ctrl) {
                  return (
                    <button key={ctrl.name}
                        className="command-btn btn btn-xs btn-warning"
                        data-mid={mid} data-ctrl={JSON.stringify(ctrl)}
                        onClick={this.onCommandClicked}>
                      {ctrl.name}
                    </button>
                  );
                }.bind(this))
              }
              </div>
            );
          }
          if (control.type == "upload") {
            return (
              <input type="file" className="file"
               data-browse-label={control.name}
               data-browse-icon="" data-show-preview="false"
               data-show-caption="false" data-show-upload="false"
               data-show-remove="false" data-browse-class={btnClasses}
               data-mid={mid} data-ctrl={JSON.stringify(control)}
               onChange={this.onUploadButtonChanged} />
            );
          } else if (control.type == "link") {
            var data = {
              'host': location.hostname,
              'port': location.port,
              'client': client
            };
            var url = $.templates(control.url).render(data);
            return (
              <a key={control.name} className={"command-btn " + btnClasses}
               href={url} target="_blank">
                {control.name}
              </a>
            );
          } else {
            return (
              <div key={control.name}
               className={"command-btn " + btnClasses}
               data-mid={mid} data-ctrl={JSON.stringify(control)}
               onClick={this.onCommandClicked}>
                {control.name}
              </div>
            );
          }
        }.bind(this))
      }
      </div>
    );
  }
});

var MainLog = React.createClass({
  appendLog: function (text) {
    var odiv = this.odiv;
    var innerHTML = $(odiv).html();

    text = this.props.fixture.extractUIMessages(text);
    innerHTML += text.replace(/\n/g, "<br />");
    if (innerHTML.length > LOG_BUF_SIZE) {
      innerHTML = innerHTML.substr(innerHTML.length -
                                   LOG_BUF_SIZE, LOG_BUF_SIZE);
    }
    $(odiv).html(innerHTML);
    odiv.scrollTop = odiv.scrollHeight;
  },
  componentDidMount: function () {
    this.odiv = this.refs["log-" + this.props.id];
  },
  render: function () {
    return (
      <div className="log log-main well well-sm" ref={"log-" + this.props.id}>
      </div>
    );
  }
});

var AuxLogs = React.createClass({
  render: function () {
    var client = this.props.client;
    var logs = client.properties.ui.logs || [];
    return (
      <div className="log-block">
        {
          logs.map(function (filename) {
            return (
              <AuxLog key={filename} mid={client.mid} filename={filename}
               fixture={this.props.fixture}/>
            )
          }.bind(this))
        }
      </div>
    );
  }
});

var AuxLog = React.createClass({
  componentDidMount: function () {
    var url = "ws" + ((window.location.protocol == "https:")? "s": "" ) +
              "://" + window.location.host + "/api/agent/shell/" +
              this.props.mid + "?command=" +
              encodeURIComponent("tail -f " + this.props.filename);
    var sock = new WebSocket(url);

    sock.onopen = function () {
      var odiv = this.refs["log-" + this.props.mid];
      sock.onmessage = function (msg) {
        if (msg.data instanceof Blob) {
          ReadBlobAsText(msg.data, function (text) {
            var innerHTML = $(odiv).html();
            text = this.props.fixture.extractUIMessages(text);
            innerHTML += text.replace(/\n/g, "<br />");
            if (innerHTML.length > LOG_BUF_SIZE) {
              innerHTML = innerHTML.substr(innerHTML.length -
                                           LOG_BUF_SIZE, LOG_BUF_SIZE);
            }
            $(odiv).html(innerHTML);
            odiv.scrollTop = odiv.scrollHeight;
          }.bind(this));
        }
      }.bind(this)
    }.bind(this)
    this.sock = sock;
  },
  componentWillUnmount: function() {
    this.sock.close();
  },
  render: function () {
    return (
      <div className="log log-aux well well-sm" ref={"log-" + this.props.mid}>
      </div>
    );
  }
});
