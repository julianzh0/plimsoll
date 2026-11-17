class Store {
  int resolve(List<int> xs, bool ok) {
    for (final x in xs) {
      if (x > 0) {
        while (ok) {
          if (x > 1) {
            return 1;
          }
        }
      }
    }
    return 0;
  }
}

int chain(int a) {
  if (a == 1) {
    return 1;
  } else if (a == 2) {
    return 2;
  } else if (a == 3) {
    return 3;
  } else {
    return 4;
  }
}
