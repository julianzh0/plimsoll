#include <assert.h>

#include "tally.h"

int main(void) {
  assert(tally_add(2));
  assert(tally_total() == 2);
  return 0;
}
