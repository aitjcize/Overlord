# Overlord iOS App

A mobile client for the Overlord dashboard, allowing you to manage your clients, run terminal commands, view camera feeds, and track file uploads from your iOS device.

## Features

- **Authentication**: Secure login with token-based authentication
- **Client Management**: View and search connected clients
- **Terminal Access**: Create and manage terminal sessions for clients
- **Camera Feeds**: View camera feeds from clients with camera capability
- **File Upload Tracking**: Monitor file upload progress in real-time
- **Settings**: Configure server address and manage account

## Requirements

- iOS 14.0+
- Xcode 12.0+
- Swift 5.3+
- An Overlord server instance

## Setup

1. Clone this repository
2. Open the project in Xcode
3. Configure the server address in the app settings
4. Build and run the app on your iOS device or simulator

## Usage

### Login

Enter your Overlord server credentials to log in. The app will save your authentication token for future sessions.

### Managing Clients

- View all connected clients in the Clients tab
- Search for specific clients using the search bar
- Tap on a client to view details and available actions

### Terminal Sessions

- Create a terminal session from a client's detail view
- Send commands and view output in real-time
- Manage active terminals in the Terminals tab

### Camera Feeds

- View camera feeds from clients with camera capability
- Take snapshots and record video (if supported)
- Manage active camera feeds in the Cameras tab

### Settings

- Configure the server address
- View app and device information
- Log out of your account

## Architecture

The app follows the MVVM (Model-View-ViewModel) architecture pattern:

- **Models**: Data structures representing clients, terminals, cameras, etc.
- **Views**: SwiftUI views for the user interface
- **ViewModels**: Business logic and data management
- **Services**: API and WebSocket communication with the server

## License

This project is licensed under the BSD License - see the LICENSE file for details. 