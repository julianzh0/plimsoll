fn route(kind: u8, on: bool) -> u8 {
    match kind {
        1 => {
            if on {
                loop {
                    break;
                }
            }
            1
        }
        _ => 0,
    }
}

fn chain(a: u8) -> u8 {
    if a == 1 {
        1
    } else if a == 2 {
        2
    } else if a == 3 {
        3
    } else {
        4
    }
}
