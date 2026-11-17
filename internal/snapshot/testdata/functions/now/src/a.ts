export function pick(a: number, b: boolean): number {
  if (a > 1) {
    return b ? 1 : 2;
  } else if (a > 0 && b) {
    return 3;
  }
  return 0;
}

export const arrow = (xs: number[]) => {
  for (const x of xs) {
    if (x) {
      xs.forEach((y) => {
        if (y) {
          return;
        }
      });
    }
  }
};

class Store {
  save = () => {
    if (this) {
      return 1;
    }
  };
  load(id: string) {
    switch (id) {
      case "a":
        return 1;
      default:
        return 2;
    }
  }
}
