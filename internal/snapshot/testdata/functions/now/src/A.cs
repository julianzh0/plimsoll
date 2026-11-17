class A {
  int Pick(int a, bool b) {
    foreach (var x in new[] { a }) {
      if (x > 1 && b) {
        try {
          return 1;
        } catch (System.Exception) {
          return 2;
        }
      }
    }
    return 0;
  }

  int Chain(int a) {
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
}
