int outer(int a) {
  int inner(int b) {
    if (b > 1) {
      if (b > 2) {
        if (b > 3) {
          return 1;
        }
      }
    }
    return 0;
  }
  return inner(a);
}
