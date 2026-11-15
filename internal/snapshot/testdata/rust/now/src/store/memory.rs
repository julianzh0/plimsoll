use super::Store;
use crate::{model::Note, store};

#[derive(Default)]
pub struct MemoryStore {
    notes: Vec<Note>,
}

impl Store for MemoryStore {
    fn add(&mut self, note: Note) {
        self.notes.push(note);
        let _ = store::count();
    }
}

#[cfg(test)]
mod tests {
    use super::{MemoryStore, Store};
    use crate::model::Note;

    #[test]
    fn adds_a_note() {
        let mut store = MemoryStore::default();
        store.add(Note::new("a"));
        assert_eq!(store.notes.len(), 1);
    }
}
