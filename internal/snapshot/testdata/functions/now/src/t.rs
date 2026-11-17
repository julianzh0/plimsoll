fn sound() -> u8 {
    1
}

#[cfg(test)]
mod tests {
    fn deep(a: u8) -> u8 {
        if a > 1 {
            if a > 2 {
                if a > 3 {
                    if a > 4 {
                        return 1;
                    }
                }
            }
        }
        0
    }
}
