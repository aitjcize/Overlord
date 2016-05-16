// Copyright 2016 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// View for TerminalWindow:
// - UploadProgressWidget
//   - [(ProgressBar|ProgressBar) ...]

var UploadProgressWidget = React.createClass({
  getInitialState: function () {
    return {records: []};
  },
  // Perform upload
  // Args:
  //  uploadRequestPath: the upload HTTP request path
  //  file: the javascript File object
  //  dest: destination filename, set to "undefined" if sid is set
  //  sid: terminal session ID, use to identify upload destionation
  //  done: callback to execute when upload is completed
  upload: function (uploadRequestPath, file, dest, sid, done) {
    var $this = this;
    var query = "?";

    if (typeof(sid) != "undefined") {
      query += "terminal_sid=" + sid;
    }

    if (typeof(dest) != "undefined") {
      query += "&dest=" + dest;
    }

    function postFile(file) {
      var id = randomID();
      var formData = new FormData();
      formData.append("file", file);

      $this.addRecord({filename: file.name, id: id});
      $.ajax({
        xhr: function () {
          var xhr = new window.XMLHttpRequest();
          xhr.upload.addEventListener("progress", function (event) {
            if (event.lengthComputable) {
              var percentComplete = Math.round(event.loaded * 100 /
                                               event.total);
              $("#" + id).css("width", percentComplete + "%");
              $("#" + id + " > .percent").text(percentComplete + "%");
            }
          }, false);
          return xhr;
        },
        url: uploadRequestPath + query,
        data: formData,
        cache: false,
        contentType: false,
        processData: false,
        type: "POST",
        success: function (data) {
          $("#" + id).css("width", "100%");
          // Display the progressbar for 1 more seconds after complete.
          setTimeout(function () {
            $this.removeRecord(id);
          }.bind(this), 1000);
        },
        complete: function () {
          // Execute done callback
          if (typeof(done) != "undefined") {
            done();
          }
        },
        error: function (data) {
          var response = JSON.parse(data.responseText);
          $this.addRecord(
              {error: true, filename: file.name, id: id,
                message: response.error});
          setTimeout(function () {
            $this.removeRecord(id);
          }.bind(this), 1000);
        }
      });
    };

    // Send GET to the API to do pre-check
    $.ajax({
      url: uploadRequestPath + query + "&filename=" + file.name,
      success: function (file) {
        return function (data) {
          // Actually upload the file
          postFile(file);
        };
      }(file),
      error: function (file) {
        return function (data) {
          var id = randomID();
          var response = JSON.parse(data.responseText);

          $this.addRecord(
              {error: true, filename: file.name, id: id,
               message: response.error});
        }
      }(file),
      type: "GET"
    });
  },
  addRecord: function (record) {
    this.setState(function (state, props) {
      state.records.push(record);
    });
  },
  removeRecord: function (id) {
    this.setState(function (state, props) {
      var index = state.records.findIndex(function (el, index, array) {
        return el.id == id;
      });
      if (index !== -1) {
        state.records.splice(index, 1);
      }
    });
  },
  render: function () {
    var display = "";
    if (this.state.records.length == 0) {
      display = "upload-progress-bars-hidden";
    }
    return (
        <div className={"upload-progress-bars panel panel-warning " + display}>
          <div className="panel-heading">Upload Progress</div>
          <div className="panel-body upload-progress-panel-body">
          {
            this.state.records.map(function (record) {
              if (record.error) {
                return <ErrorBar progress={this} record={record} />;
              } else {
                return <ProgressBar record={record} />;
              }
            }.bind(this))
          }
          </div>
        </div>
    );
  }
});

var ProgressBar = React.createClass({
  render: function () {
    return (
        <div className="progress">
          <div className="progress-bar upload-progress-bar"
            id={this.props.record.id} role="progressbar"
            aria-valuenow="0" aria-valuemin="0" aria-valuemax="100">
            <span className="percent">0%</span> - {this.props.record.filename}
          </div>
        </div>
    );
  }
});

var ErrorBar = React.createClass({
  onCloseClicked: function () {
    this.props.progress.removeRecord(this.props.record.id);
  },
  render: function () {
    return (
        <div className="progress-error">
          <div className="alert alert-danger upload-alert">
            <button type="button" className="close" aria-label="Close"
              onClick={this.onCloseClicked}>
              <span aria-hidden="true">&times;</span>
            </button>
            <b>{this.props.record.filename}</b><br />
              {this.props.record.message}
          </div>
        </div>
    );
  }
});
