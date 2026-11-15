using System;
using Acme.Notes.Store;

namespace Acme.Notes.Tests.Support;

public class FixedClock : Clock
{
    public override DateTime Now => new(2026, 1, 1);
}
