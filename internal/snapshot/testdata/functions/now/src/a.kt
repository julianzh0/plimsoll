fun pick(a: Int, b: Boolean): Int {
    if (a > 1) {
        return 1
    } else if (a > 0 && b) {
        return 2
    }
    when (a) {
        0 -> return 3
        else -> return 4
    }
}

fun chain(a: Int): Int {
    if (a == 1) {
        return 1
    } else if (a == 2) {
        return 2
    } else if (a == 3) {
        return 3
    } else {
        return 4
    }
}
