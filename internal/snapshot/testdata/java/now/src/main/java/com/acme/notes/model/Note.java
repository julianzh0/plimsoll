package com.acme.notes.model;

public record Note(String text) {
  public static final Note EMPTY = new Note("");

  public record Draft(String text) {}
}
