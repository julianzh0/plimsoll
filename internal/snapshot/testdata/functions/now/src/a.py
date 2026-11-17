def branches(a, b, c):
    if a and (b or c):
        return 1
    elif b:
        return 2
    else:
        return 3


def loops(xs):
    for x in xs:
        try:
            while x:
                x -= 1
        except ValueError:
            pass
    else:
        return 0

    def inner(y):
        if y:
            return 1
    return inner(xs)
