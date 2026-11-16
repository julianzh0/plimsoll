#include "buffer.h"

static int values[TALLY_CAPACITY];
static int used;

int buffer_push(int value) {
  if (used == TALLY_CAPACITY) {
    return 0;
  }
  values[used++] = value;
  return 1;
}
