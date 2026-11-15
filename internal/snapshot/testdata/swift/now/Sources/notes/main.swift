import ArgumentParser
import NotesCore

struct Notes: ParsableCommand {
    func run() {
        print(Store().count)
    }
}

Notes.main()
