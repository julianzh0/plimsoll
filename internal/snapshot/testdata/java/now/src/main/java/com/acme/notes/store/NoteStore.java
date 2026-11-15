package com.acme.notes.store;

import com.acme.notes.model.*;
import com.acme.notes.model.Note.Draft;
import java.util.ArrayList;

public class NoteStore {
  private final ArrayList<Note> notes = new ArrayList<>();

  void add(Draft draft) {
    notes.add(new Note(draft.text()));
  }

  int size() {
    return notes.size();
  }
}
