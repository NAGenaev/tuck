import { InputBase, PasswordInput, Select, TextInput } from '@mantine/core'

import classes from './inputs.module.css'

const shared = {
    classNames: { label: classes.label, input: classes.input },
    defaultProps: { radius: 'md' }
}

export default {
    InputBase: InputBase.extend(shared),
    PasswordInput: PasswordInput.extend(shared),
    TextInput: TextInput.extend(shared),
    Select: Select.extend(shared)
}
