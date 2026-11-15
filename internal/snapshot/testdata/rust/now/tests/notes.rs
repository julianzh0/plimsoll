mod common;

use notes_app::store::{memory::MemoryStore, Store};

#[test]
fn stores_a_note() {
    let mut store = MemoryStore::default();
    store.add(common::note());
}
