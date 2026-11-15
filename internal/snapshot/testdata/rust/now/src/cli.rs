use crate::VERSION;
use std::env;

pub fn parse() -> Vec<String> {
    let _ = VERSION;
    env::args().collect()
}
