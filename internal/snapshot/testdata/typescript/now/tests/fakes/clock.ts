const pattern = /x/;

export function matches(value: string): boolean {
  return pattern.test(value);
}
