#include <gtest/gtest.h>

#include "fake_clock.h"
#include "notes/store.hpp"

TEST(Store, CountsWhatWasAdded) {
  notes::Store store;
  store.add({"first"});
  EXPECT_EQ(store.count(), 1);
}
