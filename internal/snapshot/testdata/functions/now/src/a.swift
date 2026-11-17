func pick(a: Int, b: Bool) -> Int {
    guard a > 0 else {
        return 0
    }
    if a > 1 {
        return 1
    } else if b {
        return 2
    }
    return 3
}

func chain(a: Int) -> Int {
    if a == 1 {
        return 1
    } else if a == 2 {
        return 2
    } else if a == 3 {
        return 3
    } else if a == 4 {
        return 4
    } else {
        return 5
    }
}
