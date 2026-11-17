struct S;
impl S {
    fn save(&self) { let f = |x: u8| x; f(1); }
}
fn top() { fn inner() {} inner(); }
