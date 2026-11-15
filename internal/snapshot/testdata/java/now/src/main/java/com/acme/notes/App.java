package com.acme.notes;

import static com.acme.notes.model.Note.EMPTY;

import com.acme.notes.gen.NoteProto;
import com.acme.notes.store.NoteStore;
import java.util.List;

public class App {
  public static void main(String[] args) {
    List<Object> parts = List.of(EMPTY, new NoteStore(), NoteProto.class);
    System.out.println(parts.size());
  }
}
