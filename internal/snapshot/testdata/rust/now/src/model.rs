#[derive(Clone, Debug, Default, PartialEq)]
pub struct Note {
    pub title: String,
}

impl Note {
    pub fn new(title: &str) -> Note {
        Note { title: title.to_string() }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const TEMPLATE: &str = "\
fn main() {
}
";

    #[test]
    fn keeps_its_title() {
        assert_eq!(Note::new("a").title, "a");
        assert!(TEMPLATE.contains('}'));
    }
}
