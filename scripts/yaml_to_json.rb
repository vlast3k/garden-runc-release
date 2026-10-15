require 'json'
require 'psych'

def validate_node(node)
  if node.is_a?(Psych::Nodes::Mapping)
    keys = {}
    node.children.each_slice(2) do |key, value|
      raise 'YAML mapping keys must be scalars' unless key.is_a?(Psych::Nodes::Scalar)
      raise "duplicate YAML key: #{key.value}" if keys.key?(key.value)
      raise 'YAML merge keys are unsupported' if key.value == '<<'
      keys[key.value] = true
      validate_node(value)
    end
  elsif node.respond_to?(:children) && node.children
    node.children.each { |child| validate_node(child) }
  end
end

begin
  source = STDIN.read
  stream = Psych.parse_stream(source)
  raise 'expected exactly one YAML document' unless stream.children.length == 1
  validate_node(stream)
  value = Psych.safe_load(source, permitted_classes: [], permitted_symbols: [], aliases: false)
  raise 'YAML document must be a mapping' unless value.is_a?(Hash)
  puts JSON.generate(value)
rescue StandardError => error
  warn error.message
  exit 1
end