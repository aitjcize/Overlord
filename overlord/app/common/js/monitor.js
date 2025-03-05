// Copyright 2024 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

class MonitorWebSocket {
    constructor() {
        this.eventHandlers = new Map();
        this.connect();
    }

    connect() {
        // Use secure WebSocket if the page is served over HTTPS
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/api/monitor`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onmessage = (event) => {
            const message = JSON.parse(event.data);
            const handlers = this.eventHandlers.get(message.event) || [];
            // Pass the first data item directly since that's what handlers expect
            const data = message.data && message.data.length > 0 ? message.data[0] : null;
            handlers.forEach(handler => handler(data));
        };

        this.ws.onclose = () => {
            // Reconnect after 1 second
            setTimeout(() => this.connect(), 1000);
        };
    }

    on(event, callback) {
        if (!this.eventHandlers.has(event)) {
            this.eventHandlers.set(event, []);
        }
        this.eventHandlers.get(event).push(callback);
    }
}
