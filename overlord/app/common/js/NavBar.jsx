// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// Navigation bar for overlord apps.

var NavBar = React.createClass({
  loadAppsFromServer: function () {
    $.ajax({
      url: this.props.url,
      dataType: "json",
      success: function (data) {
        this.setState(data);
      }.bind(this),
      error: function (xhr, status, err) {
        console.error(this.props.url, status, err.toString());
      }.bind(this)
    });
  },
  getInitialState: function () {
    return {apps: []};
  },
  componentDidMount: function () {
    this.loadAppsFromServer();
  },
  render: function () {
    return (
      <nav className="navbar navbar-default navbar-static-top">
        <div className="container-fluid">
          <div className="navbar-header">
            <a className="navbar-brand" href="">Overlord::{this.props.name}</a>
          </div>
          <div className="collapse navbar-collapse navbar-right" id="bs-example-navbar-collapse-1">
            <ul className="nav navbar-nav">
              <li className="dropdown">
                <a href="#" className="dropdown-toggle" data-toggle="dropdown" role="button"
                  aria-expanded="false">Switch Apps <span className="caret"></span></a>
                <ul className="dropdown-menu" role="menu">
                {
                  this.state.apps.map(function (app) {
                    return (
                      <li key={app}><a href={"/" + app}>{app}</a></li>
                    );
                  })
                }
                </ul>
              </li>
            </ul>
          </div>
        </div>
      </nav>
    );
  }
});
