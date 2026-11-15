/* The notes library.
/* A comment inside a comment. */
use crate::gone;
*/
pub mod config;
mod model;
pub mod store;

pub use model::Note;
use store::memory::MemoryStore;

pub fn open() -> MemoryStore {
    MemoryStore::default()
}
