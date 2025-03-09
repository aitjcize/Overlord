@testable import app_ios
import XCTest

final class ClientTests: XCTestCase {
    func testClientInitialization() {
        // Given
        let mid = "test-client-id"
        let name = "Test Client"

        // When
        let client = Client(mid: mid, name: name)

        // Then
        XCTAssertEqual(client.mid, mid)
        XCTAssertEqual(client.name, name)
        XCTAssertNil(client.properties)
        XCTAssertFalse(client.pinned)
        XCTAssertFalse(client.hasCamera)
    }

    func testClientWithProperties() {
        // Given
        let mid = "test-client-id"
        let properties = ["os": "macOS", "version": "14.0"]

        // When
        var client = Client(mid: mid, properties: properties)

        // Then
        XCTAssertEqual(client.mid, mid)
        XCTAssertEqual(client.properties?["os"], "macOS")
        XCTAssertEqual(client.properties?["version"], "14.0")
    }

    func testClientWithCamera() {
        // Given
        let mid = "test-client-id"
        let properties = ["has_camera": "true"]

        // When
        let client = Client(mid: mid, properties: properties)

        // Then
        XCTAssertTrue(client.hasCamera)
    }

    func testClientPinning() {
        // Given
        let mid = "test-client-id"

        // When
        var client = Client(mid: mid)
        client.pinned = true

        // Then
        XCTAssertTrue(client.pinned)
    }

    func testClientEquality() {
        // Given
        let client1 = Client(mid: "client1")
        let client2 = Client(mid: "client1")
        let client3 = Client(mid: "client2")

        // Then
        XCTAssertEqual(client1, client2)
        XCTAssertNotEqual(client1, client3)
    }
}
