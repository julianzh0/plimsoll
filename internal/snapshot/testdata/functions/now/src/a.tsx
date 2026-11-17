export function Panel(props: { items: string[]; open: boolean }) {
  if (!props.open) {
    return null;
  }
  return (
    <ul onClick={() => { if (props.open) { close(); } }}>
      {props.open && props.items.length > 0 ? <li>first</li> : null}
      {props.items.map((item) => (item ? <li>{item}</li> : null))}
    </ul>
  );
}
