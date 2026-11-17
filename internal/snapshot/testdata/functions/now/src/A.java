class A {
  int pick(int a, boolean b) {
    if (a > 1) {
      return b ? 1 : 2;
    } else if (a > 0 || b) {
      return 3;
    } else {
      return 0;
    }
  }
}
