#include "tally.h"

#include <stdlib.h>

#include "buffer.h"

static int total;

int tally_add(int amount) {
  const char *limit = getenv("TALLY_LIMIT");
  if (limit != NULL && total + amount > atoi(limit)) {
    return 0;
  }
  total += amount;
  return buffer_push(amount);
}

int tally_total(void) { return total; }
