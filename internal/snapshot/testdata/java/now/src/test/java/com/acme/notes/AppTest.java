package com.acme.notes;

import static org.junit.jupiter.api.Assertions.assertNotNull;

import com.acme.notes.store.NoteStore;
import org.junit.jupiter.api.Test;

class AppTest {
  @Test
  void starts() {
    assertNotNull(new NoteStore());
  }
}
