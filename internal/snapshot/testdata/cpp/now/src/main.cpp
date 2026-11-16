#include <iostream>

#include <notes/store.hpp>

int main() {
  notes::Store store;
  store.add({"first"});
  std::cout << store.count() << "\n";
  return 0;
}
