#pragma once

#include <vector>

#include "note.h"

namespace notes {
class Store {
 public:
  void add(Note note);
  int count() const;

 private:
  std::vector<Note> notes_;
};
}  // namespace notes
