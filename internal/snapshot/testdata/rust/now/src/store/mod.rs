pub mod memory;

use crate::Note;

pub trait Store {
    fn add(&mut self, note: Note);
}

pub fn count() -> usize {
    0
}
