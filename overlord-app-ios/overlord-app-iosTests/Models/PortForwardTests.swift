@testable import overlord_app_ios
import XCTest

final class PortForwardTests: XCTestCase {
    func testPortForwardInitialization() {
        // Given
        let clientId = "test-client-id"
        let clientName = "Test Client"
        let remoteHost = "example.com"
        let remotePort = 80
        let localPort = 8080
        let useHttps = true

        // When
        let portForward = PortForward(
            clientId: clientId,
            clientName: clientName,
            remoteHost: remoteHost,
            remotePort: remotePort,
            localPort: localPort,
            useHttps: useHttps
        )

        // Then
        XCTAssertEqual(portForward.clientId, clientId)
        XCTAssertEqual(portForward.clientName, clientName)
        XCTAssertEqual(portForward.remoteHost, remoteHost)
        XCTAssertEqual(portForward.remotePort, remotePort)
        XCTAssertEqual(portForward.localPort, localPort)
        XCTAssertEqual(portForward.useHttps, useHttps)
        XCTAssertFalse(portForward.isActive)
        XCTAssertNil(portForward.webSocket)
    }

    func testPortForwardActivation() {
        // Given
        var portForward = PortForward(
            clientId: "client1",
            clientName: "Client",
            remoteHost: "example.com",
            remotePort: 80,
            localPort: 8080
        )

        // When
        portForward.isActive = true

        // Then
        XCTAssertTrue(portForward.isActive)
    }

    func testPortForwardLocalURL() {
        // Given
        let portForward1 = PortForward(
            clientId: "client1",
            clientName: "Client",
            remoteHost: "example.com",
            remotePort: 80,
            localPort: 8080,
            useHttps: false
        )

        let portForward2 = PortForward(
            clientId: "client2",
            clientName: "Client",
            remoteHost: "secure.example.com",
            remotePort: 443,
            localPort: 8443,
            useHttps: true
        )

        // Then
        XCTAssertEqual(portForward1.localURLString, "http://localhost:8080")
        XCTAssertEqual(portForward2.localURLString, "https://localhost:8443")

        // Also test the URL objects
        XCTAssertNotNil(portForward1.localURL)
        XCTAssertNotNil(portForward2.localURL)
        XCTAssertEqual(portForward1.localURL?.absoluteString, "http://localhost:8080")
        XCTAssertEqual(portForward2.localURL?.absoluteString, "https://localhost:8443")
    }

    func testPortForwardEquality() {
        // Given
        let id1 = UUID().uuidString
        let id2 = UUID().uuidString

        let portForward1 = PortForward(
            id: id1,
            clientId: "client1",
            clientName: "Client",
            remoteHost: "example.com",
            remotePort: 80,
            localPort: 8080
        )

        let portForward2 = PortForward(
            id: id2,
            clientId: "client1",
            clientName: "Client",
            remoteHost: "example.com",
            remotePort: 80,
            localPort: 8080
        )

        let portForward3 = PortForward(
            id: id1,
            clientId: "client1",
            clientName: "Client",
            remoteHost: "example.com",
            remotePort: 80,
            localPort: 8080
        )

        // Then - port forwards with different IDs should not be equal
        XCTAssertNotEqual(portForward1, portForward2)
        XCTAssertNotEqual(portForward1.id, portForward2.id)

        // Port forwards with the same ID should be equal
        XCTAssertEqual(portForward1, portForward3)
    }
}
