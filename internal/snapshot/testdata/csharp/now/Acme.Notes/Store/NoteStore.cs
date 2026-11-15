using System.Collections.Generic;
using Acme.Notes.Mapping;
using Acme.Notes.Model;
using static Acme.Notes.Model.Note;

namespace Acme.Notes.Store
{
    public class NoteStore
    {
        private readonly List<Note> notes = new();
        private readonly Clock clock;

        public NoteStore(Clock clock) => this.clock = clock;

        public int Count => notes.Count;

        public void Add(Note note)
        {
            using var writer = new System.IO.StringWriter();
            writer.Write(clock.Now);
            notes.Add(note);
        }
    }
}
