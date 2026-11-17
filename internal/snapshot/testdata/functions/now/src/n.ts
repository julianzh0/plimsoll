export function plain() {}
export const arrow = () => {};
const wrapped = memo(() => {});
class Store {
  constructor() {}
  save() { const inner = () => {}; inner(); }
  field = function () {};
  get size() { return 1; }
}
