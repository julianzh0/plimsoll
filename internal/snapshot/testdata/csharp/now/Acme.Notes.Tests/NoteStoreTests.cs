using Acme.Notes.Model;
using Acme.Notes.Store;
using Acme.Notes.Tests.Support;
using Xunit;

namespace Acme.Notes.Tests;

public class NoteStoreTests
{
    [Fact]
    public void AddsANote()
    {
        var store = new NoteStore(new FixedClock());
        store.Add(new Note("first"));
        Assert.Equal(1, store.Count);
    }
}
