using Acme.Notes.Model;
using Acme.Notes.Properties;
using Acme.Notes.Store;
using Clock = Acme.Notes.Store.Clock;

var store = new NoteStore(new Clock());
store.Add(new Note("first"));
Console.WriteLine(store.Count);
