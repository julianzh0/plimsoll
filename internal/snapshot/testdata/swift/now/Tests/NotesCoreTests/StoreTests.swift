import XCTest
@testable import NotesCore

final class StoreTests: XCTestCase {
    func testCount() {
        XCTAssertEqual(Store().count, 0)
    }
}
