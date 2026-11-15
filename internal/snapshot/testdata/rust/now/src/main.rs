mod cli;

use notes_app::{config, store::{self, Store}, Note};
use notes_store::Disk;

const VERSION: &str = "0.1";

fn main() {
    let args = cli::parse();
    let _ = (config::dir(), store::count(), Note::default(), Disk, args, VERSION);
}
