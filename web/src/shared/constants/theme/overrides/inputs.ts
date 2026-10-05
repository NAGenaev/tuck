// SPDX-License-Identifier: AGPL-3.0-only
import {
    Autocomplete,
    Combobox,
    InputBase,
    InputWrapper,
    MultiSelect,
    NumberInput,
    PasswordInput,
    Pill,
    PillsInput,
    Select,
    TagsInput,
    Textarea,
    TextInput
} from '@mantine/core'

import comboClasses from './combobox.module.css'
import classes from './inputs.module.css'
import { dropdownTransition, EASE } from './transitions'

const field = {
    root: classes.root,
    label: classes.label,
    required: classes.required,
    description: classes.description,
    error: classes.error,
    wrapper: classes.wrapper,
    input: classes.input,
    section: classes.section
}

const combo = {
    dropdown: comboClasses.dropdown,
    options: comboClasses.options,
    option: comboClasses.option,
    empty: comboClasses.empty,
    groupLabel: comboClasses.groupLabel,
    search: comboClasses.search
}

const shared = {
    classNames: field,
    defaultProps: { radius: 'md' as const, size: 'md' as const }
}

const comboTransition = { transition: dropdownTransition, duration: 200, timingFunction: EASE }

const withCombo = {
    classNames: { ...field, ...combo, pill: classes.pill },
    defaultProps: {
        radius: 'md' as const,
        size: 'md' as const,
        comboboxProps: { transitionProps: comboTransition }
    }
}

export default {
    InputBase: InputBase.extend(shared),
    InputWrapper: InputWrapper.extend({
        classNames: {
            root: classes.root,
            label: classes.label,
            required: classes.required,
            description: classes.description,
            error: classes.error
        }
    }),
    TextInput: TextInput.extend(shared),
    Textarea: Textarea.extend(shared),
    PasswordInput: PasswordInput.extend({
        ...shared,
        classNames: {
            ...field,
            innerInput: classes.innerInput,
            visibilityToggle: classes.visibilityToggle
        }
    }),
    NumberInput: NumberInput.extend({
        ...shared,
        classNames: { ...field, control: classes.control }
    }),
    Select: Select.extend(withCombo),
    MultiSelect: MultiSelect.extend({
        ...withCombo,
        classNames: { ...withCombo.classNames, inputField: classes.pillsField }
    }),
    TagsInput: TagsInput.extend({
        ...withCombo,
        classNames: { ...withCombo.classNames, inputField: classes.pillsField }
    }),
    Autocomplete: Autocomplete.extend(withCombo),
    PillsInput: PillsInput.extend({
        ...shared,
        classNames: field
    }),
    PillsInputField: PillsInput.Field.extend({
        classNames: { field: classes.pillsField }
    }),
    Pill: Pill.extend({
        classNames: { root: classes.pill, remove: classes.pillRemove }
    }),
    Combobox: Combobox.extend({
        classNames: combo,
        defaultProps: { transitionProps: comboTransition }
    })
}
