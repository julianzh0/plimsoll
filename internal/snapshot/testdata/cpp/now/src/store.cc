/*
Usage:
#include "notes/legacy.h"
*/
#include "notes/store.hpp"

#include <cstdlib>
#include <utility>

#include "clock.h"

#if defined(_WIN32)
#  include <windows.h>
#endif

namespace notes {
long now() { return 0; }

void Store::add(Note note) {
  if (std::getenv("NOTES_READ_ONLY") == nullptr) {
    notes_.push_back(std::move(note));
  }
}

int Store::count() const { return static_cast<int>(notes_.size()); }
}  // namespace notes
