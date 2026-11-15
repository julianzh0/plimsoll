const notes: string[] = [];

export function store(note: string): void {
  notes.push(note);
}
export { notes };
