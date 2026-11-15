package com.acme.notes.store;

import com.acme.notes.model.Note;
import org.junit.jupiter.params.ParameterizedTest;

class NoteStoreTest {
  @ParameterizedTest
  void adds(String text) {
    new NoteStore().add(new Note.Draft(text));
  }
}
