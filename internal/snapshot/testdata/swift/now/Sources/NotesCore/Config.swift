import Foundation

public func notesDir() -> String {
    ProcessInfo.processInfo.environment["NOTES_DIR"] ?? ""
}

let template = """
import Gone
"""
