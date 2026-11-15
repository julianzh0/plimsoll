using System;

namespace Acme.Notes.Store;

public class Clock
{
    public virtual DateTime Now => DateTime.UtcNow;
}
